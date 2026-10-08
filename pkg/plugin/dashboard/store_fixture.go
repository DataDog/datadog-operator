// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"

	"github.com/DataDog/datadog-operator/pkg/plugin/helm"
	"github.com/DataDog/datadog-operator/pkg/rollout"
)

// FixtureKind is the kind of the optional metadata document of a fixture.
// Its apiVersion is FixtureAPIVersion.
const (
	FixtureAPIVersion = "fixture.dashboard.datadoghq.com/v1"
	FixtureKind       = "Fixture"
)

// fixtureMeta is the metadata document of a fixture. Everything in it is
// optional.
type fixtureMeta struct {
	Taken   time.Time                    `json:"taken"`
	Cluster fixtureCluster               `json:"cluster"`
	Sources map[string]fixtureSource     `json:"sources"`
	Helm    map[string][]fixtureRevision `json:"helm"`
}

type fixtureCluster struct {
	Context       string `json:"context"`
	ServerVersion string `json:"serverVersion"`
	PluginVersion string `json:"pluginVersion"`
}

type fixtureSource struct {
	State    SourceState `json:"state"`
	Mode     RefreshMode `json:"mode"`
	NextPoll time.Time   `json:"nextPoll"`
	Notice   string      `json:"notice"`
	Err      string      `json:"err"`
}

type fixtureRevision struct {
	Chart        string    `json:"chart"`
	ChartVersion string    `json:"chartVersion"`
	Revision     int       `json:"revision"`
	Status       string    `json:"status"`
	LastDeployed time.Time `json:"lastDeployed"`
}

// FixtureBuildConfig returns the build configuration of a fixture scenario
// under pkg/plugin/dashboard/testdata, shared by the dashboard and renderer
// tests.
func FixtureBuildConfig(name string) BuildConfig {
	switch name {
	case "stalled":
		return BuildConfig{Pods: true, StallAfter: 10 * time.Minute}
	default:
		return BuildConfig{}
	}
}

// FixtureVariant is a fixture scenario built with a non-default
// configuration, for the dashboard and renderer goldens.
type FixtureVariant struct {
	// Name names the goldens of the variant.
	Name string
	// Scenario is the fixture directory under pkg/plugin/dashboard/testdata.
	Scenario string
	Config   BuildConfig
}

// FixtureVariants are the fixture variants with goldens.
var FixtureVariants = []FixtureVariant{
	{Name: "stale-daps-hide-empty", Scenario: "stale-daps", Config: BuildConfig{HideEmpty: true}},
	{Name: "stale-daps-sort-desired", Scenario: "stale-daps", Config: BuildConfig{Sort: SortDesired}},
	{Name: "stale-daps-hide-empty-sort-desired", Scenario: "stale-daps", Config: BuildConfig{HideEmpty: true, Sort: SortDesired}},
	{Name: "stale-daps-hide-empty-force", Scenario: "stale-daps", Config: BuildConfig{HideEmptyForce: true}},
	{Name: "stale-warnings-hide-empty-force", Scenario: "stale-warnings", Config: BuildConfig{HideEmptyForce: true}},
	{Name: "availability-max-unavailable-1", Scenario: "availability", Config: BuildConfig{MaxUnavailable: rollout.MaxUnavailable{Value: 1}}},
	{Name: "availability-max-unavailable-1pct", Scenario: "availability", Config: BuildConfig{MaxUnavailable: rollout.MaxUnavailable{Value: 1, Percent: true}}},
}

// FixtureStore is a test Store serving a snapshot loaded from YAML files.
// Each document is an object routed to its source by kind, except an
// optional Fixture document with the cluster info, source states and Helm
// histories. Sources default to OK; summary kinds without objects default to
// NotInstalled; pods are only present when the fixture has some or sets
// their state.
type FixtureStore struct {
	dir  string
	snap *Snapshot
}

var _ Store = &FixtureStore{}

// NewFixtureStore returns a store reading every *.yaml file of dir.
func NewFixtureStore(dir string) *FixtureStore {
	return &FixtureStore{dir: dir}
}

// Start implements Store. It loads the fixture.
func (f *FixtureStore) Start(context.Context) error {
	snap, err := LoadFixture(f.dir)
	if err != nil {
		return err
	}
	f.snap = snap
	return nil
}

// Snapshot implements Store.
func (f *FixtureStore) Snapshot() *Snapshot { return f.snap }

// Changed implements Store. A fixture never changes.
func (f *FixtureStore) Changed() <-chan struct{} { return nil }

// Close implements Store.
func (f *FixtureStore) Close() {}

// LoadFixture reads every *.yaml file of dir into a Snapshot.
func LoadFixture(dir string) (*Snapshot, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no fixture files in %s", dir)
	}
	slices.Sort(files)

	snap := &Snapshot{
		Objects: map[string][]*unstructured.Unstructured{},
		Helm:    map[string][]helm.Revision{},
		Sources: map[string]SourceInfo{},
	}
	var meta fixtureMeta
	for _, file := range files {
		if err := loadFixtureFile(file, snap, &meta); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
	}
	applyFixtureMeta(snap, meta)
	return snap, nil
}

func loadFixtureFile(file string, snap *Snapshot, meta *fixtureMeta) error {
	r, err := os.Open(file)
	if err != nil {
		return err
	}
	defer r.Close()

	sources := Sources()
	dec := utilyaml.NewYAMLOrJSONDecoder(r, 4096)
	for {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		// Unstructured decoding keeps integers as int64, as the API
		// clients do.
		obj := &unstructured.Unstructured{}
		if err := obj.UnmarshalJSON(raw); err != nil {
			return err
		}
		doc := obj.Object
		if obj.GetAPIVersion() == FixtureAPIVersion && obj.GetKind() == FixtureKind {
			if err := yaml.Unmarshal(raw, meta); err != nil {
				return fmt.Errorf("invalid fixture document: %w", err)
			}
			continue
		}
		if obj.GetKind() == "Pod" {
			pod := &corev1.Pod{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(doc, pod); err != nil {
				return fmt.Errorf("pod %s: %w", obj.GetName(), err)
			}
			stripped, _ := StripPod(pod)
			snap.Pods = append(snap.Pods, stripped.(*corev1.Pod))
			continue
		}
		src, ok := fixtureSourceFor(sources, obj)
		if !ok {
			return fmt.Errorf("no source for %s %s", obj.GetKind(), obj.GetName())
		}
		transformed, err := src.Transform(obj)
		if err != nil {
			return err
		}
		snap.Objects[src.Key] = append(snap.Objects[src.Key], transformed.(*unstructured.Unstructured))
	}
}

// fixtureSourceFor routes an object to its source by kind. Deployments
// labeled as the operator go to the operator source.
func fixtureSourceFor(sources []Source, obj *unstructured.Unstructured) (Source, bool) {
	isOperator := obj.GetLabels()[appNameLabel] == operatorAppName
	for _, src := range sources {
		if src.Kind != obj.GetKind() {
			continue
		}
		if src.Kind == "Deployment" && (src.Key == SourceOperator) != isOperator {
			continue
		}
		return src, true
	}
	return Source{}, false
}

func applyFixtureMeta(snap *Snapshot, meta fixtureMeta) {
	snap.Taken = meta.Taken
	snap.Cluster = ClusterInfo(meta.Cluster)
	for key, revs := range meta.Helm {
		out := make([]helm.Revision, len(revs))
		for i, r := range revs {
			out[i] = helm.Revision(r)
		}
		snap.Helm[key] = out
	}
	for _, src := range Sources() {
		info := SourceInfo{State: SourceOK, Mode: RefreshOnce, LastOK: meta.Taken}
		switch {
		case src.Key == SourcePods && snap.Pods == nil:
			info.State = ""
		case IsSummarySource(src.Key) && len(snap.Objects[src.Key]) == 0:
			info = SourceInfo{State: SourceNotInstalled, Mode: RefreshOnce}
		}
		if m, ok := meta.Sources[src.Key]; ok {
			info.State, info.Notice, info.Err, info.NextPoll = m.State, m.Notice, m.Err, m.NextPoll
			if m.Mode != "" {
				info.Mode = m.Mode
			}
			if info.State != SourceOK {
				info.LastOK = time.Time{}
			}
		}
		if info.State != "" {
			snap.Sources[src.Key] = info
		}
	}
}
