package gcp_agent

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

const maxGCPNameLength = 30
const truncatedHashLength = 6
const maxTruncatedLength = maxGCPNameLength - truncatedHashLength - 1 // add another char for the hyphen
