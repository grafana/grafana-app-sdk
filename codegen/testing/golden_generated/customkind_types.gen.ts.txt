interface InnerObject1 {
  innerField1: string;
  innerField2: string[];
  innerField3: Array<InnerObject2>;
}

const defaultInnerObject1: Partial<InnerObject1> = {
  innerField2: [],
  innerField3: [],
};

interface InnerObject2 {
  details: Record<string, unknown>;
  name: string;
}

interface Type1 {
  group: string;
  options?: string[];
}

const defaultType1: Partial<Type1> = {
  options: [],
};

interface Type2 {
  details: Record<string, unknown>;
  group: string;
}

type UnionType = (Type1 | Type2);

export interface CustomKind {
  /**
   * metadata contains embedded CommonMetadata and can be extended with custom string fields
   * TODO: use CommonMetadata instead of redefining here; currently needs to be defined here
   * without external reference as using the CommonMetadata reference breaks thema codegen.
   */
  metadata: {
    /**
     * All extensions to this metadata need to have string values (for APIServer encoding-to-annotations purposes)
     * Can't use this as it's not yet enforced CUE:
     * ...string
     * Have to do this gnarly regex instead
     */
    customMetadataField: string;
    updateTimestamp: string;
    createdBy: string;
    updatedBy: string;
    uid: string;
    creationTimestamp: string;
    deletionTimestamp?: string;
    finalizers: string[];
    resourceVersion: string;
    /**
     * All extensions to this metadata need to have string values (for APIServer encoding-to-annotations purposes)
     * Can't use this as it's not yet enforced CUE:
     * ...string
     * Have to do this gnarly regex instead
     */
    otherMetadataField: string;
    /**
     * extraFields is reserved for any fields that are pulled from the API server metadata but do not have concrete fields in the CUE metadata
     */
    extraFields: Record<string, unknown>;
    labels: Record<string, string>;
  };
  spec: {
    field1: string;
    inner: InnerObject1;
    union: UnionType;
    map: Record<string, Type2>;
    timestamp: string;
    enum: ('val1' | 'val2' | 'val3' | 'val4' | 'default');
    i32: number;
    i64: number;
    boolField: boolean;
    floatField: number;
  };
  status: {
    statusField1: string;
    /**
     * operatorStates is a map of operator ID to operator state evaluations.
     * Any operator which consumes this kind SHOULD add its state evaluation information to this field.
     */
    operatorStates?: Record<string, {
  /**
   * lastEvaluation is the ResourceVersion last evaluated
   */
  lastEvaluation: string,
  /**
   * state describes the state of the lastEvaluation.
   * It is limited to three possible states for machine evaluation.
   */
  state: ('success' | 'in_progress' | 'failed'),
  /**
   * descriptiveState is an optional more descriptive state field which has no requirements on format
   */
  descriptiveState?: string,
  /**
   * details contains any extra information that is operator-specific
   */
  details?: Record<string, unknown>,
}>;
    /**
     * additionalFields is reserved for future use
     */
    additionalFields?: Record<string, unknown>;
  };
}
