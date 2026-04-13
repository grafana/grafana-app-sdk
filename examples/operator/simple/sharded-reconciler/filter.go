package main

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"time"

	kitlog "github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/dskit/dns"
	"github.com/grafana/dskit/flagext"
	"github.com/grafana/dskit/kv"
	"github.com/grafana/dskit/kv/codec"
	"github.com/grafana/dskit/kv/memberlist"
	"github.com/grafana/dskit/netutil"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/services"

	"github.com/grafana/grafana-app-sdk/resource"
)

const (
	shardRingName = "resource-shard-filter"
	shardRingKey  = "resource-shard-filter"
)

var shardRingOp = ring.WriteNoExtend

type HashRingShardFilterConfig struct {
	InstanceID         string
	InstanceAddr       string
	NumTokens          int
	HeartbeatPeriod    time.Duration
	HeartbeatTimeout   time.Duration
	RejoinInterval     time.Duration
	MemberlistBindAddr string
	MemberlistBindPort int

	MemberlistAdvertiseAddr string
	MemberlistAdvertisePort int

	MemberlistJoinMembers []string
	AbortIfJoinFails      bool

	MemberlistClusterLabel                     string
	MemberlistClusterLabelVerificationDisabled bool
}

type shardRingReader interface {
	GetWithOptions(key uint32, op ring.Operation, opts ...ring.Option) (ring.ReplicationSet, error)
	State() services.State
}

type shardRingInstance interface {
	GetInstanceAddr() string
}

type HashRingShardFilter struct {
	readRing shardRingReader
	instance shardRingInstance
	manager  *services.Manager
}

func NewHashRingShardFilter(cfg HashRingShardFilterConfig, logger kitlog.Logger, reg prometheus.Registerer) (*HashRingShardFilter, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	// dskit's ring, memberlist, and DNS helpers require a go-kit logger. The rest of the
	// example can continue to use the SDK's normal logging patterns independently.
	memberlistCfg := &memberlist.KVConfig{}
	flagext.DefaultValues(memberlistCfg)
	memberlistCfg.Codecs = []codec.Codec{ring.GetCodec()}
	memberlistCfg.ClusterLabel = cfg.MemberlistClusterLabel
	memberlistCfg.ClusterLabelVerificationDisabled = cfg.MemberlistClusterLabelVerificationDisabled
	memberlistCfg.JoinMembers = cfg.MemberlistJoinMembers
	memberlistCfg.AbortIfJoinFails = cfg.AbortIfJoinFails
	memberlistCfg.RejoinInterval = cfg.RejoinInterval
	memberlistCfg.AdvertiseAddr = cfg.MemberlistAdvertiseAddr
	memberlistCfg.AdvertisePort = cfg.instancePort()
	memberlistCfg.TCPTransport.BindPort = cfg.MemberlistBindPort
	if cfg.MemberlistBindAddr != "" {
		memberlistCfg.TCPTransport.BindAddrs = []string{cfg.MemberlistBindAddr}
	}

	dnsProviderReg := prometheus.WrapRegistererWith(prometheus.Labels{
		"component": shardRingName,
	}, reg)
	dnsProvider := dns.NewProvider(logger, dnsProviderReg, dns.GolangResolverType)

	memberlistService := memberlist.NewKVInitService(memberlistCfg, logger, dnsProvider, reg)

	ringStoreCfg := kv.Config{Store: "memberlist"}
	ringStoreCfg.MemberlistKV = memberlistService.GetMemberlistKV

	ringStore, err := kv.NewClient(
		ringStoreCfg,
		ring.GetCodec(),
		kv.RegistererWithKVName(reg, shardRingName),
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("create shard ring KV client: %w", err)
	}

	readRing, err := ring.NewWithStoreClientAndStrategy(
		ring.Config{
			KVStore:           ringStoreCfg,
			HeartbeatTimeout:  cfg.HeartbeatTimeout,
			ReplicationFactor: 1,
		},
		shardRingName,
		shardRingKey,
		ringStore,
		ring.NewIgnoreUnhealthyInstancesReplicationStrategy(),
		reg,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("create shard ring: %w", err)
	}

	instanceAddr, err := cfg.resolveInstanceAddr(logger)
	if err != nil {
		return nil, err
	}

	delegate := ring.BasicLifecyclerDelegate(ring.NewInstanceRegisterDelegate(ring.ACTIVE, cfg.NumTokens))
	delegate = ring.NewLeaveOnStoppingDelegate(delegate, logger)
	delegate = ring.NewAutoForgetDelegate(cfg.HeartbeatTimeout*2, delegate, logger)

	lifecycler, err := ring.NewBasicLifecycler(
		ring.BasicLifecyclerConfig{
			ID:               cfg.instanceID(),
			Addr:             fmt.Sprintf("%s:%d", instanceAddr, cfg.instancePort()),
			HeartbeatPeriod:  cfg.HeartbeatPeriod,
			HeartbeatTimeout: cfg.HeartbeatTimeout,
			NumTokens:        cfg.NumTokens,
		},
		shardRingName,
		shardRingKey,
		ringStore,
		delegate,
		logger,
		reg,
	)
	if err != nil {
		return nil, fmt.Errorf("create shard lifecycler: %w", err)
	}

	manager, err := services.NewManager(memberlistService, readRing, lifecycler)
	if err != nil {
		return nil, fmt.Errorf("create shard manager: %w", err)
	}

	return &HashRingShardFilter{
		readRing: readRing,
		instance: lifecycler,
		manager:  manager,
	}, nil
}

func (f *HashRingShardFilter) Run(ctx context.Context) error {
	if err := services.StartManagerAndAwaitHealthy(ctx, f.manager); err != nil {
		return err
	}

	<-ctx.Done()

	stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return services.StopManagerAndAwaitStopped(stopCtx, f.manager)
}

func (f *HashRingShardFilter) ShouldProcess(_ context.Context, obj resource.Object) (bool, error) {
	if obj == nil {
		return false, errors.New("object is required")
	}
	if state := f.readRing.State(); state != services.Running {
		return false, fmt.Errorf("shard ring is not running: %s", state)
	}

	replicationSet, err := f.readRing.GetWithOptions(hashShardKey(obj), shardRingOp)
	if err != nil {
		return false, fmt.Errorf("resolve shard assignment: %w", err)
	}

	return replicationSet.Includes(f.instance.GetInstanceAddr()), nil
}

func (cfg HashRingShardFilterConfig) validate() error {
	if cfg.NumTokens <= 0 {
		return errors.New("num tokens must be greater than zero")
	}
	if cfg.HeartbeatPeriod <= 0 {
		return errors.New("heartbeat period must be greater than zero")
	}
	if cfg.HeartbeatTimeout <= 0 {
		return errors.New("heartbeat timeout must be greater than zero")
	}
	if cfg.instancePort() <= 0 {
		return errors.New("memberlist port must be greater than zero")
	}

	return nil
}

func (cfg HashRingShardFilterConfig) instanceID() string {
	if cfg.InstanceID != "" {
		return cfg.InstanceID
	}

	hostname, err := os.Hostname()
	if err != nil {
		return shardRingName
	}

	return hostname
}

func (cfg HashRingShardFilterConfig) instancePort() int {
	if cfg.MemberlistAdvertisePort > 0 {
		return cfg.MemberlistAdvertisePort
	}

	return cfg.MemberlistBindPort
}

func (cfg HashRingShardFilterConfig) resolveInstanceAddr(logger kitlog.Logger) (string, error) {
	instanceAddr := cfg.InstanceAddr
	if instanceAddr == "" {
		instanceAddr = cfg.MemberlistAdvertiseAddr
	}

	addr, err := ring.GetInstanceAddr(
		instanceAddr,
		netutil.PrivateNetworkInterfacesWithFallback([]string{"eth0", "en0"}, logger),
		logger,
		true,
	)
	if err != nil {
		return "", fmt.Errorf("resolve shard instance address: %w", err)
	}

	return addr, nil
}

func hashShardKey(obj resource.Object) uint32 {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(obj.GroupVersionKind().Kind))
	_, _ = hasher.Write([]byte("/"))
	_, _ = hasher.Write([]byte(obj.GetNamespace()))
	_, _ = hasher.Write([]byte("/"))
	_, _ = hasher.Write([]byte(obj.GetName()))
	return hasher.Sum32()
}
