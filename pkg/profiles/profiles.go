package profiles

import (
	"fmt"
	"sync"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ProfilesV2 struct {
	// dda    *datadoghqv2alpha1.DatadogAgent
	merged map[string]*datadoghqv2alpha1.DatadogAgent
	// <dda-name>-<dap-name>
	// map[string]*datadoghqv2alpha1.DatadogAgent{
	// 	"foo-bar": &datadoghqv2alpha1.DatadogAgent{
	// 		Name: aos,
	// 	},
	// 	"foo2-bar2": &datadoghqv2alpha1.DatadogAgent{},
	// }
	// nodeMap map[string]string // todo
	defaultAffinity map[string][]corev1.NodeSelectorRequirement // for default profile?

	log   logr.Logger
	mutex sync.Mutex
}

func NewProfilesV2(log logr.Logger) ProfilesV2 {
	return ProfilesV2{
		merged:          make(map[string]*datadoghqv2alpha1.DatadogAgent),
		defaultAffinity: make(map[string][]corev1.NodeSelectorRequirement),
		log:             log,
	}
}

func (pv2 *ProfilesV2) MergeObjects(c client.Client, dda *datadoghqv2alpha1.DatadogAgent, dap *datadoghqv1alpha1.DatadogAgentProfile) *datadoghqv2alpha1.DatadogAgent {
	ddaCopy := dda.DeepCopy()
	// need to find a good way to merge

	return ddaCopy
}

func (pv2 *ProfilesV2) Add(dda *datadoghqv2alpha1.DatadogAgent) {
	pv2.mutex.Lock()
	defer pv2.mutex.Unlock()

	// todo: store namespaced name
	pv2.merged[dda.GetName()] = dda
	fmt.Printf("Add dda name: %+v, dda: %+v\n", dda.GetName(), pv2.merged[dda.GetName()])
}

func (pv2 *ProfilesV2) GetAllDDA() map[string]*datadoghqv2alpha1.DatadogAgent {
	pv2.mutex.Lock()
	defer pv2.mutex.Unlock()

	return pv2.merged
}

func (pv2 *ProfilesV2) GetDDAByNamespacedName(namespacedName types.NamespacedName) *datadoghqv2alpha1.DatadogAgent {
	pv2.mutex.Lock()
	defer pv2.mutex.Unlock()

	// todo: use namespaced name
	if dda, ok := pv2.merged[namespacedName.Name]; ok {
		return dda
	}
	return nil
}

func GetMergedDDAName(ddaName, dapName string) string {
	return fmt.Sprintf("%s-%s", ddaName, dapName)
}

func GenerateNodeAffinity(nsr []corev1.NodeSelectorRequirement, dda *datadoghqv2alpha1.DatadogAgent) {
	if dda.Spec.Override == nil {
		dda.Spec.Override = map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
			datadoghqv2alpha1.NodeAgentComponentName: {},
		}
	}
	ddaAffinity := dda.Spec.Override[datadoghqv2alpha1.NodeAgentComponentName].Affinity
	// todo?: deduplicate affinity code
	if ddaAffinity == nil {
		ddaAffinity = &corev1.Affinity{}
	}
	if ddaAffinity.NodeAffinity == nil {
		ddaAffinity.NodeAffinity = &corev1.NodeAffinity{}
	}
	if ddaAffinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		ddaAffinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{}
	}
	// NodeSelectorTerms are ORed
	if len(ddaAffinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms) == 0 {
		ddaAffinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = append(
			ddaAffinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms,
			corev1.NodeSelectorTerm{},
		)
	}
	// NodeSelectorTerms are ANDed
	if ddaAffinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions == nil {
		ddaAffinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions = []corev1.NodeSelectorRequirement{}
	}
	ddaAffinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions = append(
		ddaAffinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions,
		nsr...,
	)
}

func (pv2 *ProfilesV2) GenerateDefaultAffinity(name string, dapAffinity *datadoghqv1alpha1.DAPAffinity) {
	oNSRList := []corev1.NodeSelectorRequirement{}
	for _, nsr := range dapAffinity.DAPNodeAffinity {
		oNSR := corev1.NodeSelectorRequirement{
			Key:      nsr.Key,
			Operator: getOppositeOperator(nsr.Operator),
			Values:   nsr.Values,
		}
		oNSRList = append(oNSRList, oNSR)
	}

	pv2.mutex.Lock()
	defer pv2.mutex.Unlock()
	pv2.defaultAffinity[name] = oNSRList
}

func getOppositeOperator(op corev1.NodeSelectorOperator) corev1.NodeSelectorOperator {
	switch op {
	case corev1.NodeSelectorOpIn:
		return corev1.NodeSelectorOpNotIn
	case corev1.NodeSelectorOpNotIn:
		return corev1.NodeSelectorOpIn
	case corev1.NodeSelectorOpExists:
		return corev1.NodeSelectorOpDoesNotExist
	case corev1.NodeSelectorOpDoesNotExist:
		return corev1.NodeSelectorOpExists
	case corev1.NodeSelectorOpGt:
		return corev1.NodeSelectorOpLt
	case corev1.NodeSelectorOpLt:
		return corev1.NodeSelectorOpGt
	default:
		return ""
	}
}

func (pv2 *ProfilesV2) AddDefaultAffinity(dda *datadoghqv2alpha1.DatadogAgent) {
	pv2.mutex.Lock()
	da := pv2.defaultAffinity
	pv2.mutex.Unlock()

	daList := combineDefaultAffinity(da)
	GenerateNodeAffinity(daList, dda)
}

func combineDefaultAffinity(da map[string][]corev1.NodeSelectorRequirement) []corev1.NodeSelectorRequirement {
	daList := []corev1.NodeSelectorRequirement{}
	for _, nsrs := range da {
		daList = append(daList, nsrs...)
	}
	return daList
}
