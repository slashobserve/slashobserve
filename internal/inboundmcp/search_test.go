package inboundmcp

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"
)

type fixtureTool struct {
	name        string
	description string
	params      []string
}

var kubernetesTools = []fixtureTool{
	{"pods_list", "List all the Kubernetes pods in the current cluster from all namespaces", []string{"labelSelector"}},
	{"pods_list_in_namespace", "List all the Kubernetes pods in the specified namespace in the current cluster", []string{"namespace", "labelSelector"}},
	{"pods_get", "Get a Kubernetes Pod in the current or provided namespace with the provided name", []string{"namespace", "name"}},
	{"pods_log", "Get the logs of a Kubernetes Pod in the current or provided namespace with the provided name", []string{"namespace", "name", "container", "previous", "tail"}},
	{"pods_top", "List the resource consumption (CPU and memory) as recorded by the Kubernetes Metrics Server for the specified Kubernetes Pods", []string{"namespace", "name", "all_namespaces"}},
	{"pods_exec", "Execute a command in a Kubernetes Pod in the current or provided namespace", []string{"namespace", "name", "command", "container"}},
	{"nodes_top", "List the resource consumption (CPU and memory) as recorded by the Kubernetes Metrics Server for the specified Kubernetes Nodes", []string{"name"}},
	{"nodes_log", "Get logs from a Kubernetes node: kubelet, kube-proxy or other system logs", []string{"name", "query", "tailLines"}},
	{"events_list", "List all the Kubernetes events in the current cluster from all namespaces", []string{"namespace"}},
	{"namespaces_list", "List all the Kubernetes namespaces in the current cluster", nil},
	{"resources_list", "List Kubernetes resources and objects in the current cluster by providing their apiVersion and kind and optionally the namespace and label selector", []string{"apiVersion", "kind", "namespace", "labelSelector"}},
	{"resources_get", "Get a Kubernetes resource in the current cluster by providing its apiVersion and kind and optionally the namespace and name", []string{"apiVersion", "kind", "namespace", "name"}},
	{"configuration_view", "Get the current Kubernetes configuration content as a kubeconfig YAML", []string{"minified"}},
	{"helm_list", "List all the Helm releases in the current or provided namespace", []string{"namespace", "all_namespaces"}},
	{"deployments_rollout_status", "Show whether a deployment finished rolling out its new replica set, and how many replicas are ready", []string{"namespace", "name"}},
	{"services_list", "List the Kubernetes services and their cluster IPs and ports", []string{"namespace"}},
}

var codeTools = []fixtureTool{
	{"search_code", "Search the repository's source code for a text or a regular expression and return the matching lines with file and line number", []string{"query", "path", "limit"}},
	{"read_file", "Read the contents of a file in the repository at a given revision", []string{"path", "revision"}},
	{"list_directory", "List the files and folders in a directory of the repository", []string{"path"}},
	{"find_references", "Find where a function, class or variable is used across the code base", []string{"symbol", "path"}},
	{"go_to_definition", "Find the file and line where a function, class or variable is defined", []string{"symbol", "path"}},
	{"git_log", "Show the commit history of the repository or of one file, newest first", []string{"path", "limit", "since"}},
	{"git_blame", "Show who last changed each line of a file, with the commit and the date", []string{"path", "start_line", "end_line"}},
	{"git_diff", "Show the changes between two commits, branches or tags", []string{"from", "to", "path"}},
	{"get_commit", "Get the message, author and changed files of one commit by its hash", []string{"sha"}},
	{"list_branches", "List the branches of the repository", nil},
	{"get_dependencies", "List the libraries and versions a module depends on, read from go.mod, package.json or requirements.txt", []string{"path"}},
	{"list_pull_requests", "List the pull requests that were merged into the main branch, with their authors", []string{"state", "limit"}},
}

func fixtureSet(t *testing.T) []searchable {
	t.Helper()
	var out []searchable
	for source, tools := range map[string][]fixtureTool{"cluster": kubernetesTools, "code": codeTools} {
		for _, f := range tools {
			properties := map[string]any{}
			for _, p := range f.params {
				properties[p] = map[string]any{"type": "string"}
			}
			schema, err := json.Marshal(map[string]any{"type": "object", "properties": properties})
			if err != nil {
				t.Fatal(err)
			}
			tool := Tool{Name: f.name, State: ToolApproved, Pinned: Definition{Description: f.description, InputSchema: schema}}
			out = append(out, searchableOf(Source{UID: source, Name: source}, tool))
		}
	}
	return out
}

func hitNames(hits []scored) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.source.UID + "/" + h.tool.Name
	}
	return out
}

func TestTheExpectedToolIsInTheFirstThreeForPlainWordQueries(t *testing.T) {
	set := fixtureSet(t)
	for query, want := range map[string]string{
		"logs of a crashing pod":                     "cluster/pods_log",
		"show me the pods in the checkout namespace": "cluster/pods_list_in_namespace",
		"what pods are running":                      "cluster/pods_list",
		"how much cpu and memory is a pod using":     "cluster/pods_top",
		"node memory usage":                          "cluster/nodes_top",
		"recent events in the cluster":               "cluster/events_list",
		"which namespaces exist":                     "cluster/namespaces_list",
		"did the deployment finish rolling out":      "cluster/deployments_rollout_status",
		"helm releases":                              "cluster/helm_list",
		"get a custom resource by kind":              "cluster/resources_get",
		"kubelet logs":                               "cluster/nodes_log",
		"run a command inside a container":           "cluster/pods_exec",
		"find the text checkout in the source code":  "code/search_code",
		"read the file main.go":                      "code/read_file",
		"who last changed this line":                 "code/git_blame",
		"where is this function defined":             "code/go_to_definition",
		"where is this function used":                "code/find_references",
		"what changed between two releases":          "code/git_diff",
		"commit history of a file":                   "code/git_log",
		"which libraries does the module depend on":  "code/get_dependencies",
		"list the files in a folder":                 "code/list_directory",
		"which pull requests were merged recently":   "code/list_pull_requests",
		"details of one commit by its hash":          "code/get_commit",
		"PODS_LOG":                                   "cluster/pods_log",
		"podsLog":                                    "cluster/pods_log",
	} {
		t.Run(query, func(t *testing.T) {
			hits := rank(query, set)
			if !slices.Contains(hitNames(hits[:min(3, len(hits))]), want) {
				t.Errorf("first three = %v, want %s among them", hitNames(hits[:min(3, len(hits))]), want)
			}
		})
	}
}

func TestRankingNeedsAWordOfTheToolAndIsStable(t *testing.T) {
	set := fixtureSet(t)
	for _, query := range []string{"", "   ", "the of and", "quantum entanglement", "!!!"} {
		if hits := rank(query, set); len(hits) != 0 {
			t.Errorf("%q matched %v", query, hitNames(hits))
		}
	}
	first, second := hitNames(rank("list namespace", set)), hitNames(rank("list namespace", slices.Clone(set)))
	if !slices.Equal(first, second) {
		t.Errorf("ranking is not stable: %v vs %v", first, second)
	}
}

func TestAParameterNameAloneFindsATool(t *testing.T) {
	hits := rank("label selector", fixtureSet(t))
	if len(hits) != 3 {
		t.Errorf("hits = %v", hitNames(hits))
	}
}

func TestPluralsAndCasesMatchTheSameWords(t *testing.T) {
	for _, pair := range [][2]string{{"pods", "pod"}, {"Logs", "log"}, {"policies", "policy"}, {"restarting", "restart"}, {"status", "status"}, {"class", "class"}, {"processes", "process"}, {"statuses", "status"}, {"classes", "class"}, {"boxes", "box"}, {"matches", "match"}, {"running", "run"}, {"logging", "log"}, {"calling", "call"}, {"databases", "database"}, {"nodes", "node"}} {
		if got := words(pair[0]); len(got) != 1 || got[0] != pair[1] {
			t.Errorf("words(%q) = %v, want [%s]", pair[0], got, pair[1])
		}
	}
	if got := words("getHTTPLogs"); !slices.Equal(got, []string{"get", "http", "log"}) {
		t.Errorf("words(getHTTPLogs) = %v", got)
	}
	if got := words("HTTPRoute"); !slices.Equal(got, []string{"http", "route"}) {
		t.Errorf("words(HTTPRoute) = %v", got)
	}
	if got := words("getFileContents_v2"); !slices.Equal(got, []string{"get", "file", "content", "v2"}) {
		t.Errorf("words = %v", got)
	}
}

func TestTheWholeSetStaysUsableAt300Tools(t *testing.T) {
	var set []searchable
	for i := range 300 {
		tool := Tool{Name: fmt.Sprintf("tool_%03d_widget", i), State: ToolApproved, Pinned: Definition{Description: "Handle widget number " + fmt.Sprint(i), InputSchema: json.RawMessage(`{"type":"object"}`)}}
		set = append(set, searchableOf(Source{UID: "big"}, tool))
	}
	set = append(set, searchableOf(Source{UID: "big"}, Tool{Name: "pods_log", State: ToolApproved, Pinned: Definition{Description: "Get the logs of a pod", InputSchema: json.RawMessage(`{"type":"object"}`)}}))
	if hits := rank("logs of a pod", set); len(hits) == 0 || hits[0].tool.Name != "pods_log" {
		t.Errorf("hits = %v", hitNames(hits[:min(3, len(hits))]))
	}
}
