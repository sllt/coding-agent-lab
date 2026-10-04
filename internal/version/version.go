// Package version records the product build identity.
package version

const (
	Product = "agentlab"
	// Version is the product line implemented in this tree.
	Version = "1.0.0"
	// PiCommit is the Pi revision the design reviewed. The module graph
	// resolves it; tests compare this constant with the linked module.
	PiCommit = "4c292d04b898d95862ea74e17be7adb115a4ad54"
)
