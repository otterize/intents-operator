package gcp_agent

const maxGCPNameLength = 30
const truncatedHashLength = 6
const maxTruncatedLength = maxGCPNameLength - truncatedHashLength - 1 // add another char for the hyphen
