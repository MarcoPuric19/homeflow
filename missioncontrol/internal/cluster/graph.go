package cluster

import (
	"fmt"
	"sort"
	"strings"
)

// Node kinds the cockpit draws.
const (
	KindPod       = "pod"
	KindService   = "service"
	KindVolume    = "volume"
	KindConfig    = "config"
	KindSecret    = "secret"
	KindNamespace = "namespace"
	KindOutside   = "outside"
)

// Edge kinds. Each one is derived from a real Kubernetes object, never guessed.
const (
	EdgeRoutes   = "routes"    // Service -> Pod, by label selector
	EdgeMounts   = "mounts"    // Pod -> PVC
	EdgeReads    = "reads"     // Pod -> ConfigMap/Secret
	EdgeMayReach = "may-reach" // NetworkPolicy egress
	EdgeAccepts  = "accepts"   // NetworkPolicy ingress
	EdgeOwnedBy  = "owned-by"  // Pod -> ReplicaSet/StatefulSet
)

type Node struct {
	ID        string            `json:"id"`
	Kind      string            `json:"kind"`
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Status    string            `json:"status,omitempty"`
	Healthy   bool              `json:"healthy"`
	Detail    map[string]string `json:"detail,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}

type Edge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Kind   string `json:"kind"`
	Label  string `json:"label,omitempty"`
	Source string `json:"source"` // which object this edge was read from
}

func nodeID(kind, namespace, name string) string {
	return fmt.Sprintf("%s/%s/%s", kind, namespace, name)
}

// matches reports whether labels satisfy every entry of a selector. An empty
// selector matches everything, which is how Kubernetes reads podSelector: {}.
func matches(selector, labels map[string]string) bool {
	for key, want := range selector {
		if labels[key] != want {
			return false
		}
	}
	return true
}

func podHealthy(p pod) (string, bool) {
	phase := p.Status.Phase
	ready := true
	restarts := 0
	for _, status := range p.Status.ContainerStatuses {
		if !status.Ready {
			ready = false
		}
		restarts += status.RestartCount
	}
	if len(p.Status.ContainerStatuses) == 0 {
		ready = false
	}
	label := phase
	if !ready {
		label = phase + " (nicht bereit)"
	}
	if restarts > 0 {
		label = fmt.Sprintf("%s · %d Neustarts", label, restarts)
	}
	return label, phase == "Running" && ready
}

func build(namespace string, pods []pod, services []service, policies []networkPolicy) ([]Node, []Edge) {
	var nodes []Node
	var edges []Edge
	seen := map[string]bool{}

	add := func(node Node) string {
		if !seen[node.ID] {
			seen[node.ID] = true
			nodes = append(nodes, node)
		}
		return node.ID
	}
	link := func(from, to, kind, label, source string) {
		if from == "" || to == "" {
			return
		}
		edges = append(edges, Edge{From: from, To: to, Kind: kind, Label: label, Source: source})
	}

	for _, p := range pods {
		status, healthy := podHealthy(p)
		images := make([]string, 0, len(p.Spec.Containers))
		requests := make([]string, 0, len(p.Spec.Containers))
		for _, c := range p.Spec.Containers {
			images = append(images, c.Image)
			if memory := c.Resources.Limits["memory"]; memory != "" {
				requests = append(requests, c.Name+": "+memory)
			}
		}
		detail := map[string]string{"image": strings.Join(images, ", ")}
		if len(requests) > 0 {
			detail["memoryLimit"] = strings.Join(requests, ", ")
		}
		if p.Status.PodIP != "" {
			detail["podIP"] = p.Status.PodIP
		}
		podNode := add(Node{ID: nodeID(KindPod, namespace, p.Metadata.Name), Kind: KindPod,
			Name: p.Metadata.Name, Namespace: namespace, Status: status, Healthy: healthy,
			Detail: detail, Labels: p.Metadata.Labels})

		for _, owner := range p.Metadata.OwnerReferences {
			detail := map[string]string{"kind": owner.Kind}
			ownerNode := add(Node{ID: nodeID("workload", namespace, owner.Name), Kind: "workload",
				Name: owner.Name, Namespace: namespace, Status: owner.Kind, Healthy: true, Detail: detail})
			link(podNode, ownerNode, EdgeOwnedBy, owner.Kind, "ownerReference")
		}

		for _, volume := range p.Spec.Volumes {
			switch {
			case volume.PersistentVolumeClaim != nil:
				claim := add(Node{ID: nodeID(KindVolume, namespace, volume.PersistentVolumeClaim.ClaimName),
					Kind: KindVolume, Name: volume.PersistentVolumeClaim.ClaimName, Namespace: namespace, Healthy: true})
				link(podNode, claim, EdgeMounts, volume.Name, "pod.spec.volumes")
			case volume.ConfigMap != nil:
				config := add(Node{ID: nodeID(KindConfig, namespace, volume.ConfigMap.Name),
					Kind: KindConfig, Name: volume.ConfigMap.Name, Namespace: namespace, Healthy: true})
				link(podNode, config, EdgeReads, volume.Name, "pod.spec.volumes")
			case volume.Secret != nil:
				secret := add(Node{ID: nodeID(KindSecret, namespace, volume.Secret.SecretName),
					Kind: KindSecret, Name: volume.Secret.SecretName, Namespace: namespace, Healthy: true})
				link(podNode, secret, EdgeReads, volume.Name, "pod.spec.volumes")
			}
		}

		// Configuration pulled in through the environment is a dependency too,
		// and it is the one people forget when a pod suddenly fails to start.
		for _, c := range p.Spec.Containers {
			for _, source := range c.EnvFrom {
				if source.ConfigMapRef != nil {
					link(podNode, add(Node{ID: nodeID(KindConfig, namespace, source.ConfigMapRef.Name), Kind: KindConfig,
						Name: source.ConfigMapRef.Name, Namespace: namespace, Healthy: true}), EdgeReads, "envFrom", "container.envFrom")
				}
				if source.SecretRef != nil {
					link(podNode, add(Node{ID: nodeID(KindSecret, namespace, source.SecretRef.Name), Kind: KindSecret,
						Name: source.SecretRef.Name, Namespace: namespace, Healthy: true}), EdgeReads, "envFrom", "container.envFrom")
				}
			}
			for _, env := range c.Env {
				if env.ValueFrom == nil {
					continue
				}
				if ref := env.ValueFrom.ConfigMapKeyRef; ref != nil {
					link(podNode, add(Node{ID: nodeID(KindConfig, namespace, ref.Name), Kind: KindConfig,
						Name: ref.Name, Namespace: namespace, Healthy: true}), EdgeReads, "env", "container.env")
				}
				if ref := env.ValueFrom.SecretKeyRef; ref != nil {
					link(podNode, add(Node{ID: nodeID(KindSecret, namespace, ref.Name), Kind: KindSecret,
						Name: ref.Name, Namespace: namespace, Healthy: true}), EdgeReads, "env", "container.env")
				}
			}
		}
	}

	for _, s := range services {
		ports := make([]string, 0, len(s.Spec.Ports))
		for _, port := range s.Spec.Ports {
			ports = append(ports, fmt.Sprintf("%d", port.Port))
		}
		serviceNode := add(Node{ID: nodeID(KindService, namespace, s.Metadata.Name), Kind: KindService,
			Name: s.Metadata.Name, Namespace: namespace, Status: s.Spec.Type, Healthy: true,
			Detail: map[string]string{"ports": strings.Join(ports, ", ")}})
		backends := 0
		for _, p := range pods {
			if len(s.Spec.Selector) > 0 && matches(s.Spec.Selector, p.Metadata.Labels) {
				link(serviceNode, nodeID(KindPod, namespace, p.Metadata.Name), EdgeRoutes, strings.Join(ports, ", "), "service.spec.selector")
				backends++
			}
		}
		// A Service without a single matching pod is the classic silent
		// outage, so the cockpit marks it instead of drawing nothing.
		if backends == 0 {
			for i := range nodes {
				if nodes[i].ID == serviceNode {
					nodes[i].Healthy = false
					nodes[i].Status = s.Spec.Type + " · kein Backend"
				}
			}
		}
	}

	edges = append(edges, policyEdges(namespace, pods, policies, add)...)

	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Kind != nodes[j].Kind {
			return nodes[i].Kind < nodes[j].Kind
		}
		return nodes[i].Name < nodes[j].Name
	})
	return nodes, edges
}
