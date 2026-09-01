// Package cluster turns a Kubernetes namespace into a small dependency graph.
//
// It shells out to kubectl instead of linking a Kubernetes client library: the
// tool is a hard requirement of this repository anyway (scripts/check-tools.sh),
// it already carries the user's kubeconfig and context, and the alternative
// would pull tens of megabytes of dependencies into a cockpit that is supposed
// to stay small.
package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Runner executes a kubectl invocation and returns its stdout. It exists so
// tests can feed recorded fixtures instead of talking to a cluster.
type Runner func(ctx context.Context, args ...string) ([]byte, error)

// KubectlRunner builds the runner. kubeconfig and context are explicit so
// Mission Control can watch a cluster it does not live in - including a
// dedicated one later - without depending on whatever KUBECONFIG happens to be
// set in the environment that started it.
func KubectlRunner(binary, kubeconfig, kubeContext string) Runner {
	return func(ctx context.Context, args ...string) ([]byte, error) {
		full := make([]string, 0, len(args)+4)
		if kubeconfig != "" {
			full = append(full, "--kubeconfig", kubeconfig)
		}
		if kubeContext != "" {
			full = append(full, "--context", kubeContext)
		}
		full = append(full, args...)
		return execCommand(ctx, binary, full...)
	}
}

// execCommand is a variable so tests can observe the arguments without running
// kubectl.
var execCommand = func(ctx context.Context, binary string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("kubectl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// Raw JSON shapes, reduced to the fields the graph needs.
type list[T any] struct {
	Items []T `json:"items"`
}

type meta struct {
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace"`
	Labels          map[string]string `json:"labels"`
	OwnerReferences []struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	} `json:"ownerReferences"`
}

type pod struct {
	Metadata meta `json:"metadata"`
	Spec     struct {
		Containers []container `json:"containers"`
		Volumes    []struct {
			Name                  string `json:"name"`
			PersistentVolumeClaim *struct {
				ClaimName string `json:"claimName"`
			} `json:"persistentVolumeClaim"`
			ConfigMap *struct {
				Name string `json:"name"`
			} `json:"configMap"`
			Secret *struct {
				SecretName string `json:"secretName"`
			} `json:"secret"`
		} `json:"volumes"`
	} `json:"spec"`
	Status struct {
		Phase             string `json:"phase"`
		PodIP             string `json:"podIP"`
		ContainerStatuses []struct {
			Ready        bool `json:"ready"`
			RestartCount int  `json:"restartCount"`
		} `json:"containerStatuses"`
		Conditions []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"conditions"`
	} `json:"status"`
}

type container struct {
	Name  string `json:"name"`
	Image string `json:"image"`
	Env   []struct {
		ValueFrom *struct {
			ConfigMapKeyRef *struct {
				Name string `json:"name"`
			} `json:"configMapKeyRef"`
			SecretKeyRef *struct {
				Name string `json:"name"`
			} `json:"secretKeyRef"`
		} `json:"valueFrom"`
	} `json:"env"`
	EnvFrom []struct {
		ConfigMapRef *struct {
			Name string `json:"name"`
		} `json:"configMapRef"`
		SecretRef *struct {
			Name string `json:"name"`
		} `json:"secretRef"`
	} `json:"envFrom"`
	Resources struct {
		Requests map[string]string `json:"requests"`
		Limits   map[string]string `json:"limits"`
	} `json:"resources"`
}

type service struct {
	Metadata meta `json:"metadata"`
	Spec     struct {
		Type     string            `json:"type"`
		Selector map[string]string `json:"selector"`
		Ports    []struct {
			Name string `json:"name"`
			Port int    `json:"port"`
		} `json:"ports"`
	} `json:"spec"`
}

type networkPolicy struct {
	Metadata meta `json:"metadata"`
	Spec     struct {
		PodSelector selector     `json:"podSelector"`
		PolicyTypes []string     `json:"policyTypes"`
		Ingress     []policyRule `json:"ingress"`
		Egress      []policyRule `json:"egress"`
	} `json:"spec"`
}

type selector struct {
	MatchLabels map[string]string `json:"matchLabels"`
}

type policyRule struct {
	From  []peer `json:"from"`
	To    []peer `json:"to"`
	Ports []struct {
		Protocol string `json:"protocol"`
		Port     any    `json:"port"`
	} `json:"ports"`
}

type peer struct {
	PodSelector       *selector `json:"podSelector"`
	NamespaceSelector *selector `json:"namespaceSelector"`
	IPBlock           *struct {
		CIDR string `json:"cidr"`
	} `json:"ipBlock"`
}

// Snapshot is one reading of the cluster.
type Snapshot struct {
	TakenAt    time.Time `json:"takenAt"`
	Namespaces []string  `json:"namespaces"`
	Nodes      []Node    `json:"nodes"`
	Edges      []Edge    `json:"edges"`
	Warnings   []string  `json:"warnings"`
}

// One kubectl call per namespace instead of one per resource: each invocation
// costs roughly 45 MiB and 180 ms, and the cockpit polls continuously. Asking
// for three kinds at once turns six processes per cycle into two.
func fetchAll(ctx context.Context, run Runner, namespace string) ([]pod, []service, []networkPolicy, error) {
	out, err := run(ctx, "-n", namespace, "get", "pods,services,networkpolicies", "-o", "json")
	if err != nil {
		return nil, nil, nil, err
	}
	var raw struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, nil, nil, fmt.Errorf("decode %s: %w", namespace, err)
	}
	var pods []pod
	var services []service
	var policies []networkPolicy
	for _, item := range raw.Items {
		var probe struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(item, &probe); err != nil {
			return nil, nil, nil, fmt.Errorf("decode %s: %w", namespace, err)
		}
		switch probe.Kind {
		case "Pod":
			var value pod
			if err := json.Unmarshal(item, &value); err != nil {
				return nil, nil, nil, err
			}
			pods = append(pods, value)
		case "Service":
			var value service
			if err := json.Unmarshal(item, &value); err != nil {
				return nil, nil, nil, err
			}
			services = append(services, value)
		case "NetworkPolicy":
			var value networkPolicy
			if err := json.Unmarshal(item, &value); err != nil {
				return nil, nil, nil, err
			}
			policies = append(policies, value)
		}
	}
	return pods, services, policies, nil
}

// Take reads every requested namespace. A namespace that cannot be read does
// not abort the snapshot; it becomes a warning, so one broken namespace never
// blanks the cockpit for the others.
func Take(ctx context.Context, run Runner, namespaces []string) Snapshot {
	snapshot := Snapshot{TakenAt: time.Now(), Namespaces: namespaces}
	for _, namespace := range namespaces {
		pods, services, policies, err := fetchAll(ctx, run, namespace)
		if err != nil {
			snapshot.Warnings = append(snapshot.Warnings, err.Error())
			continue
		}
		nodes, edges := build(namespace, pods, services, policies)
		snapshot.Nodes = append(snapshot.Nodes, nodes...)
		snapshot.Edges = append(snapshot.Edges, edges...)
	}
	return snapshot
}
