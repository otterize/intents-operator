package awsagent

const iamAPIVersion = "2012-10-17"
const iamEffectAllow = "Allow"

const serviceAccountNameTagKey = "otterize/serviceAccountName"
const serviceAccountNamespaceTagKey = "otterize/serviceAccountNamespace"
const clusterNameTagKey = "otterize/clusterName"
const softDeletionStrategyTagKey = "otterize/softDeletionStrategy"
const softDeletionStrategyTagValue = "true"
const softDeletedTagKey = "otterize/softDeletedAt"

const policyNameTagKey = "otterize/policyName"
const policyNamespaceTagKey = "otterize/policyNamespace"
const policyHashTagKey = "otterize/policyHash"

const iamRoleDescription = "This IAM role was created by the Otterize intents-operator's AWS integration. For more details, go to https://otterize.com"

// PolicyDocument is our definition of our policies to be uploaded to IAM.
type PolicyDocument struct {
	Version   string
	Statement []StatementEntry
}

// StatementEntry will dictate what this policy will allow or not allow.
type StatementEntry struct {
	Effect    string            `json:"Effect,omitempty"`
	Action    []string          `json:"Action,omitempty"`
	Resource  string            `json:"Resource,omitempty"`
	Principal map[string]string `json:"Principal,omitempty"`
	Sid       string            `json:"Sid,omitempty"`
	Condition map[string]any    `json:"Condition,omitempty"`
}

const maxAWSNameLength = 64
