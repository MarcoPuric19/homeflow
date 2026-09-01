package cluster

import (
	"fmt"
	"sort"
	"strings"
)

// policyEdges turns NetworkPolicies into the edges that actually describe who
// may talk to whom. On this cluster the policies are enforced (kindnetd ships
// kube-network-policies), so these edges are a control, not a wish - see
// Austausch.md, observation 7.2.
//
// Two things are deliberately visible rather than hidden:
//   - a pod covered by a policy type with no matching rule is isolated, and
//     that is drawn as an explicit "abgeriegelt" marker;
//   - a peer in another namespace is drawn as an outside node, because this
//     snapshot only knows the namespaces it was asked to read.
func policyEdges(namespace string, pods []pod, policies []networkPolicy, add func(Node) string) []Edge {
	var edges []Edge
	for _, policy := range policies {
		selected := make([]pod, 0, len(pods))
		for _, p := range pods {
			if matches(policy.Spec.PodSelector.MatchLabels, p.Metadata.Labels) {
				selected = append(selected, p)
			}
		}
		if len(selected) == 0 {
			continue
		}
		source := "networkpolicy/" + policy.Metadata.Name
		for _, policyType := range policy.Spec.PolicyTypes {
			rules := policy.Spec.Ingress
			if policyType == "Egress" {
				rules = policy.Spec.Egress
			}
			if len(rules) == 0 {
				// Default deny: the policy names the type but allows nothing.
				for _, p := range selected {
					target := add(Node{ID: nodeID(KindOutside, namespace, "abgeriegelt-"+strings.ToLower(policyType)),
						Kind: KindOutside, Name: "abgeriegelt (" + policyType + ")", Namespace: namespace, Healthy: true,
						Detail: map[string]string{"quelle": policy.Metadata.Name}})
					podNode := nodeID(KindPod, namespace, p.Metadata.Name)
					if policyType == "Egress" {
						edges = append(edges, Edge{From: podNode, To: target, Kind: EdgeMayReach, Label: "alles verboten", Source: source})
					} else {
						edges = append(edges, Edge{From: target, To: podNode, Kind: EdgeAccepts, Label: "alles verboten", Source: source})
					}
				}
				continue
			}
			for _, rule := range rules {
				label := portLabel(rule)
				peers := rule.From
				if policyType == "Egress" {
					peers = rule.To
				}
				for _, peer := range peers {
					for _, p := range selected {
						podNode := nodeID(KindPod, namespace, p.Metadata.Name)
						target := peerNode(namespace, peer, pods, add)
						if target == "" {
							continue
						}
						if policyType == "Egress" {
							edges = append(edges, Edge{From: podNode, To: target, Kind: EdgeMayReach, Label: label, Source: source})
						} else {
							edges = append(edges, Edge{From: target, To: podNode, Kind: EdgeAccepts, Label: label, Source: source})
						}
					}
				}
			}
		}
	}
	return edges
}

func portLabel(rule policyRule) string {
	parts := make([]string, 0, len(rule.Ports))
	for _, port := range rule.Ports {
		parts = append(parts, fmt.Sprintf("%v/%s", port.Port, port.Protocol))
	}
	return strings.Join(parts, ", ")
}

// peerNode resolves a policy peer to a node. A peer inside the same namespace
// resolves to the concrete pods; anything else becomes one outside node, named
// after what the policy actually says.
func peerNode(namespace string, p peer, pods []pod, add func(Node) string) string {
	switch {
	case p.IPBlock != nil:
		return add(Node{ID: nodeID(KindOutside, namespace, p.IPBlock.CIDR), Kind: KindOutside,
			Name: p.IPBlock.CIDR, Namespace: namespace, Healthy: true})
	case p.NamespaceSelector != nil:
		name := describeSelector(p.NamespaceSelector.MatchLabels)
		if p.PodSelector != nil {
			name += " / " + describeSelector(p.PodSelector.MatchLabels)
		}
		return add(Node{ID: nodeID(KindOutside, namespace, name), Kind: KindOutside,
			Name: name, Namespace: namespace, Healthy: true,
			Detail: map[string]string{"hinweis": "anderer Namespace"}})
	case p.PodSelector != nil:
		for _, candidate := range pods {
			if matches(p.PodSelector.MatchLabels, candidate.Metadata.Labels) {
				return nodeID(KindPod, namespace, candidate.Metadata.Name)
			}
		}
		// The policy names a peer that currently has no pod. That is worth
		// seeing: it is either a scaled-down workload or a stale selector.
		name := describeSelector(p.PodSelector.MatchLabels)
		return add(Node{ID: nodeID(KindOutside, namespace, "ohne-pod-"+name), Kind: KindOutside,
			Name: name + " (kein Pod)", Namespace: namespace, Healthy: false,
			Detail: map[string]string{"hinweis": "Selektor trifft derzeit keinen Pod"}})
	}
	return ""
}

// describeSelector names a selector by its values, sorted, so the same policy
// always produces the same node id across snapshots.
func describeSelector(labels map[string]string) string {
	if len(labels) == 0 {
		return "alle"
	}
	parts := make([]string, 0, len(labels))
	for _, value := range labels {
		parts = append(parts, value)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}
