# GraphQL Gateway Rate Limiting

The GraphQL Gateway supports comprehensive rate limiting to protect against abuse and ensure fair usage. The rate limiting is implemented using a token bucket algorithm and is highly configurable.

## Features

- **Token Bucket Algorithm**: Provides both sustained rate limiting and burst capacity
- **Flexible Key Extraction**: Rate limit by IP address, user, API key, or custom criteria
- **Memory Efficient**: Automatic cleanup of unused rate limiting buckets
- **Configurable**: Fine-grained control over rates, bursts, and cleanup intervals
- **GraphQL Compliant**: Returns proper GraphQL error responses with extensions

## Configuration

Rate limiting is configured through the `GatewayConfig.RateLimit` field:

```go
config := gateway.GatewayConfig{
    Logger: logger,
    RateLimit: gateway.RateLimitConfig{
        Enabled:           true,                          // Enable/disable rate limiting
        RequestsPerSecond: 100,                          // Sustained rate (tokens per second)
        BurstSize:         200,                          // Maximum burst capacity
        KeyExtractor:      gateway.DefaultKeyExtractor,  // How to identify clients
        CleanupInterval:   5 * time.Minute,             // How often to clean up unused buckets
    },
}

gw := gateway.NewFederatedGateway(config)
```

## Key Extractors

Key extractors determine how clients are identified for rate limiting purposes:

### IP-Based Rate Limiting (Default)

```go
config.RateLimit.KeyExtractor = gateway.DefaultKeyExtractor
```

Extracts client IP from:
1. `X-Forwarded-For` header (first IP in chain)
2. `X-Real-IP` header  
3. `RemoteAddr` (fallback)

### User-Based Rate Limiting

```go
config.RateLimit.KeyExtractor = gateway.UserKeyExtractor
```

Extracts rate limiting key from:
1. `Authorization` header (prefixed with "user:")
2. Client IP (fallback)

### Custom Key Extraction

```go
customExtractor := func(r *http.Request) string {
    // Rate limit by API key
    if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
        return "api:" + apiKey
    }
    // Rate limit by organization
    if org := r.Header.Get("X-Org-ID"); org != "" {
        return "org:" + org
    }
    // Fall back to IP
    return gateway.DefaultKeyExtractor(r)
}
config.RateLimit.KeyExtractor = customExtractor
```

## Algorithm Details

The token bucket algorithm works as follows:

1. **Initial State**: Each client starts with a full bucket (BurstSize tokens)
2. **Token Refill**: Tokens are added at the configured rate (RequestsPerSecond)
3. **Request Processing**: Each request consumes one token
4. **Rate Limiting**: Requests are rejected when no tokens are available

### Example: 100 RPS with 200 Burst

- Sustained rate: 100 requests per second
- Burst capacity: 200 requests can be made immediately
- After burst: Must wait for tokens to refill at 100/second

## Error Responses

When rate limited, clients receive a proper GraphQL error response:

```json
{
  "errors": [
    {
      "message": "Rate limit exceeded. Too many requests.",
      "extensions": {
        "code": "RATE_LIMITED",
        "timestamp": 1640995200
      }
    }
  ]
}
```

HTTP Response Headers:
- `Status: 429 Too Many Requests`
- `Retry-After: 60`
- `Content-Type: application/json`

## Administrative Functions

### Check Rate Limit Status

```go
enabled, config := gateway.GetRateLimitStatus()
if enabled {
    fmt.Printf("Rate limiting enabled: %.1f RPS, %d burst\n", 
               config.RequestsPerSecond, config.BurstSize)
}
```

### Reset Rate Limit for Client

```go
// Reset rate limit for specific IP
gateway.ResetRateLimit("192.168.1.100")

// Reset rate limit for specific user
gateway.ResetRateLimit("user:Bearer token123")
```

## Performance Considerations

- **Memory Usage**: Each unique client key uses ~100 bytes of memory
- **Cleanup**: Unused buckets are automatically cleaned up (default: 5 minutes)
- **Concurrency**: The implementation is fully thread-safe with minimal lock contention
- **CPU Overhead**: Rate limiting adds ~1-5μs per request

## Best Practices

1. **Set Reasonable Limits**: Start with generous limits and tighten based on usage patterns
2. **Monitor Usage**: Log rate limiting events to understand client behavior
3. **Communicate Limits**: Document rate limits in your API documentation
4. **Handle Bursts**: Set burst size to 2-5x the sustained rate for typical workloads
5. **Cleanup Tuning**: Adjust cleanup interval based on client diversity

## Example Configurations

### Development/Testing
```go
RateLimitConfig{
    Enabled:           true,
    RequestsPerSecond: 1000,    // Very generous
    BurstSize:         2000,
    KeyExtractor:      DefaultKeyExtractor,
    CleanupInterval:   time.Minute,
}
```

### Production API
```go
RateLimitConfig{
    Enabled:           true,
    RequestsPerSecond: 100,     // Reasonable for most APIs
    BurstSize:         300,
    KeyExtractor:      UserKeyExtractor,
    CleanupInterval:   10 * time.Minute,
}
```

### High-Traffic Service
```go
RateLimitConfig{
    Enabled:           true,
    RequestsPerSecond: 50,      // More restrictive
    BurstSize:         100,
    KeyExtractor:      customExtractor, // Multi-tier limiting
    CleanupInterval:   5 * time.Minute,
}
```

## Disable Rate Limiting

To disable rate limiting entirely:

```go
config := gateway.GatewayConfig{
    Logger: logger,
    RateLimit: gateway.RateLimitConfig{
        Enabled: false,  // All other fields ignored when disabled
    },
}
```

## Testing

The implementation includes comprehensive tests covering:

- Token bucket algorithm correctness
- Key extraction strategies  
- HTTP integration
- Administrative functions
- Edge cases and error conditions

Run tests with:
```bash
cd graphql/gateway && go test -v
``` 
