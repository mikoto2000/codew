package chatloop

import "ollama-codex-cli/internal/tools"

type Decision string

const (
	DecisionAllowed                Decision = "allowed"
	DecisionDenied                 Decision = "denied"
	DecisionNeedsUserApproval      Decision = "needs-user-approval"
	DecisionNeedsNetworkEscalation Decision = "needs-network-escalation"
)

type ApprovalRequest struct {
	ToolName     string
	IsMCP        bool
	IsMutating   bool
	Sandbox      string
	AutoApprove  bool
	NetworkAllow bool
	NetworkRules map[string]bool
	Profile      string
}

func Decide(req ApprovalRequest) Decision {
	if req.ToolName == "" {
		return DecisionDenied
	}
	if !req.IsMCP && !tools.IsToolAllowed(req.Profile, req.ToolName) {
		return DecisionDenied
	}
	if needsNetworkEscalation(req) {
		return DecisionNeedsNetworkEscalation
	}
	if req.IsMutating && !req.AutoApprove {
		return DecisionNeedsUserApproval
	}
	return DecisionAllowed
}

func NeedsNetworkEscalation(req ApprovalRequest) bool {
	return needsNetworkEscalation(req)
}

func needsNetworkEscalation(req ApprovalRequest) bool {
	if !tools.RequiresNetwork(req.ToolName, req.IsMCP) {
		return false
	}
	if tools.AllowsNetwork(req.Sandbox) {
		return false
	}
	if req.NetworkAllow {
		return false
	}
	return !req.NetworkRules[req.ToolName]
}
