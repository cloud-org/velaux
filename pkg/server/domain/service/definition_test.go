/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package service

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/schema"

	v1 "github.com/kubevela/velaux/pkg/server/interfaces/api/dto/v1"
)

var _ = Describe("Test namespace service functions", func() {
	BeforeEach(func() {
		InitTestEnv("todo")
		err := k8sClient.Create(context.Background(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-system",
			},
		})
		Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})
	It("Test ListDefinitions function", func() {
		By("List component definitions")
		webserver, err := os.ReadFile("./testdata/webserver-cd.yaml")
		Expect(err).Should(Succeed())
		var cd v1beta1.ComponentDefinition
		err = yaml.Unmarshal(webserver, &cd)
		Expect(err).Should(Succeed())
		err = k8sClient.Create(context.Background(), &cd)
		Expect(err).Should(Succeed())

		definitions, err := definitionService.ListDefinitions(context.TODO(), DefinitionQueryOption{Type: "component"})
		Expect(err).Should(BeNil())
		var selectDefinition *v1.DefinitionBase
		for i, definition := range definitions {
			if definition.WorkloadType == "deployments.apps" {
				selectDefinition = definitions[i]
			}
		}
		Expect(selectDefinition).ShouldNot(BeNil())
		Expect(cmp.Diff(selectDefinition.Name, "webservice-test")).Should(BeEmpty())
		Expect(selectDefinition.Description).ShouldNot(BeEmpty())
		Expect(selectDefinition.Alias).Should(Equal("test-alias"))

		By("List trait definitions")
		myingress, err := os.ReadFile("./testdata/myingress-td.yaml")
		Expect(err).Should(Succeed())
		var td v1beta1.TraitDefinition
		err = yaml.Unmarshal(myingress, &td)
		Expect(err).Should(Succeed())
		err = k8sClient.Create(context.Background(), &td)
		Expect(err).Should(Succeed())
		traits, err := definitionService.ListDefinitions(context.TODO(), DefinitionQueryOption{Type: "trait"})
		Expect(err).Should(BeNil())
		// there is already a scaler trait definition in the test env
		Expect(cmp.Diff(len(traits), 2)).Should(BeEmpty())
		Expect(cmp.Diff(traits[0].Name, "myingress")).Should(BeEmpty())
		// The OwnAddon field of myingress should not be fluxcd
		Expect(traits[0].OwnerAddon).Should(Equal("fluxcd"))
		Expect(traits[0].Description).ShouldNot(BeEmpty())
		Expect(traits[0].Trait).ShouldNot(BeNil())
		Expect(traits[0].Alias).Should(Equal("test-alias"))

		By("List workflow step definitions")
		step, err := os.ReadFile("./testdata/applyapplication-sd.yaml")
		Expect(err).Should(Succeed())
		var sd v1beta1.WorkflowStepDefinition
		err = yaml.Unmarshal(step, &sd)
		Expect(err).Should(Succeed())
		err = k8sClient.Create(context.Background(), &sd)
		Expect(err).Should(Succeed())

		wfstep, err := definitionService.ListDefinitions(context.TODO(), DefinitionQueryOption{Type: "workflowstep"})
		Expect(err).Should(BeNil())
		// there is already a deploy workflow step definition in the test env
		Expect(cmp.Diff(len(wfstep), 2)).Should(BeEmpty())
		Expect(cmp.Diff(wfstep[0].Name, "apply-application")).Should(BeEmpty())
		Expect(wfstep[0].Description).ShouldNot(BeEmpty())
		Expect(wfstep[0].WorkflowStep.Schematic).ShouldNot(BeNil())
		Expect(wfstep[0].Alias).Should(Equal("test-alias"))

		wfstep, err = definitionService.ListDefinitions(context.TODO(), DefinitionQueryOption{Type: "workflowstep", Scope: "WorkflowRun"})
		Expect(err).Should(BeNil())
		// the definition should be filtered
		Expect(cmp.Diff(len(wfstep), 1)).Should(BeEmpty())

		step, err = os.ReadFile("./testdata/apply-application-hide.yaml")
		Expect(err).Should(Succeed())
		var sd2 v1beta1.WorkflowStepDefinition
		err = yaml.Unmarshal(step, &sd2)
		Expect(err).Should(Succeed())
		err = k8sClient.Create(context.Background(), &sd2)
		Expect(err).Should(Succeed())

		allstep, err := definitionService.ListDefinitions(context.TODO(), DefinitionQueryOption{Type: "workflowstep", QueryAll: true})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(allstep), 3)).Should(BeEmpty())

		By("List policy definitions")
		var policy = v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "health",
				Namespace: "vela-system",
				Annotations: map[string]string{
					"definition.oam.dev/description": "this is a policy definition",
					"definition.oam.dev/alias":       "test-alias",
				},
			},
			Spec: v1beta1.PolicyDefinitionSpec{
				ManageHealthCheck: true,
			},
		}
		err = k8sClient.Create(context.Background(), &policy)
		Expect(err).Should(Succeed())
		policies, err := definitionService.ListDefinitions(context.TODO(), DefinitionQueryOption{Type: "policy"})
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(policies), 1)).Should(BeEmpty())
		Expect(cmp.Diff(policies[0].Name, "health")).Should(BeEmpty())
		Expect(policies[0].Description).ShouldNot(BeEmpty())
		Expect(policies[0].Policy.ManageHealthCheck).Should(BeTrue())
		Expect(policies[0].Alias).Should(Equal("test-alias"))

		By("Filtering list by owner addon")
		list, err := definitionService.ListDefinitions(context.TODO(), DefinitionQueryOption{Type: "trait", OwnerAddon: "non-existent-addon"})
		Expect(err).Should(Succeed())
		// All results should be filtered out
		Expect(list).Should(HaveLen(0))

		list, err = definitionService.ListDefinitions(context.TODO(), DefinitionQueryOption{Type: "trait", OwnerAddon: "fluxcd"})
		Expect(err).Should(Succeed())
		// We should see myingress being kept because fluxcd is its owner
		Expect(len(list) >= 1).Should(Equal(true))
		Expect(list[0].Name).Should(Equal("myingress"))
	})

	It("Test DetailDefinition function", func() {

		webserver, err := os.ReadFile("./testdata/apply-object.yaml")
		Expect(err).Should(Succeed())
		var cd v1beta1.WorkflowStepDefinition
		err = yaml.Unmarshal(webserver, &cd)
		Expect(err).Should(Succeed())
		Expect(k8sClient.Create(context.Background(), &cd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "workflowstep-schema-apply-object",
				Namespace: "vela-system",
			},
			Data: map[string]string{
				types.OpenapiV3JSONSchema: `{"properties":{"batchPartition":{"title":"batchPartition","type":"integer"},"volumes": {"description":"Specify volume type, options: pvc, configMap, secret, emptyDir","enum":["pvc","configMap","secret","emptyDir"],"title":"volumes","type":"string"}, "rolloutBatches":{"items":{"properties":{"replicas":{"title":"replicas","type":"integer"}},"required":["replicas"],"type":"object"},"title":"rolloutBatches","type":"array"},"targetRevision":{"title":"targetRevision","type":"string"},"targetSize":{"title":"targetSize","type":"integer"}},"required":["targetRevision","targetSize"],"type":"object"}`,
			},
		}
		err = k8sClient.Create(context.Background(), cm)
		Expect(err).Should(Succeed())
		definitionDetail, err := definitionService.DetailDefinition(context.TODO(), "apply-object", "workflowstep")
		Expect(err).Should(Succeed())

		schemaFromCM := &openapi3.Schema{}
		err = schemaFromCM.UnmarshalJSON([]byte(cm.Data["openapi-v3-json-schema"]))
		Expect(err).Should(Succeed())

		Expect(definitionDetail.APISchema).Should(Equal(schemaFromCM))
		Expect(definitionDetail.WorkflowStep).ShouldNot(BeNil())
	})

	It("Test renderDefaultUISchema", func() {
		schema := &v1.DetailDefinitionResponse{}
		data, err := os.ReadFile("./testdata/api-schema.json")
		Expect(err).Should(Succeed())
		err = json.Unmarshal(data, schema)
		Expect(err).Should(Succeed())
		Expect(cmp.Diff(len(schema.APISchema.Required), 3)).Should(BeEmpty())
		uiSchema := renderDefaultUISchema(schema.APISchema)
		Expect(cmp.Diff(len(uiSchema), 12)).Should(BeEmpty())
	})

	It("Test patchSchema", func() {
		ddr := &v1.DetailDefinitionResponse{}
		data, err := os.ReadFile("./testdata/api-schema.json")
		Expect(err).Should(Succeed())
		err = json.Unmarshal(data, ddr)
		Expect(err).Should(Succeed())
		Expect(cmp.Diff(len(ddr.APISchema.Required), 3)).Should(BeEmpty())
		defaultschema := renderDefaultUISchema(ddr.APISchema)

		customschema := []*schema.UIParameter{}
		cdata, err := os.ReadFile("./testdata/ui-custom-schema.yaml")
		Expect(err).Should(Succeed())
		err = yaml.Unmarshal(cdata, &customschema)
		Expect(err).Should(Succeed())

		uiSchema := patchSchema(defaultschema, customschema)
		Expect(cmp.Diff(len(uiSchema), 12)).Should(BeEmpty())
		Expect(cmp.Diff(uiSchema[7].JSONKey, "livenessProbe")).Should(BeEmpty())
		Expect(cmp.Diff(len(uiSchema[7].SubParameters), 8)).Should(BeEmpty())
	})

	It("Test sortDefaultUISchema", testSortDefaultUISchema)

	It("Test update ui schema", func() {
		du := &definitionServiceImpl{
			KubeClient: k8sClient,
		}
		cdata, err := os.ReadFile("./testdata/workflowstep-apply-object.yaml")
		Expect(err).Should(Succeed())
		var schema schema.UISchema
		err = yaml.Unmarshal(cdata, &schema)
		Expect(err).Should(Succeed())
		uiSchema, err := du.AddDefinitionUISchema(context.TODO(), "apply-object", "workflowstep", schema)
		Expect(err).Should(Succeed())
		for _, param := range uiSchema {
			if param.JSONKey == "batchPartition" {
				Expect(len(param.Conditions)).Should(Equal(1))
				Expect(param.Validate.Required).Should(Equal(true))
				Expect(param.Sort).Should(Equal(uint(77)))
			}
		}
	})

	It("Test update status of the definition", func() {
		du := &definitionServiceImpl{
			KubeClient: k8sClient,
		}
		detail, err := du.UpdateDefinitionStatus(context.TODO(), "apply-object", v1.UpdateDefinitionStatusRequest{
			DefinitionType: "workflowstep",
			HiddenInUI:     true,
		})
		Expect(err).Should(Succeed())
		Expect(detail.Status).Should(Equal("disable"))

		detail, err = du.UpdateDefinitionStatus(context.TODO(), "apply-object", v1.UpdateDefinitionStatusRequest{
			DefinitionType: "workflowstep",
			HiddenInUI:     false,
		})
		Expect(err).Should(Succeed())
		Expect(detail.Status).Should(Equal("enable"))
	})

})

func testSortDefaultUISchema() {
	var params = []*schema.UIParameter{
		{
			Label: "P1",
			Validate: &schema.Validate{
				Required: true,
			},
			SubParameters: []*schema.UIParameter{
				{Label: "P1S1"},
			},
			Sort: 100,
		}, {
			Label: "T2",
			Validate: &schema.Validate{
				Required: true,
			},
			SubParameters: []*schema.UIParameter{
				{Label: "T2S1"},
				{Label: "T2S2"},
				{Label: "T2S3"},
			},
			Sort: 100,
		}, {
			Label: "T3",
			Validate: &schema.Validate{
				Required: false,
			},
			Sort: 100,
		}, {
			Label: "P4",
			Validate: &schema.Validate{
				Required: false,
			},
			Sort: 100,
		}, {
			Label: "T5",
			Validate: &schema.Validate{
				Required: true,
			},
			SubParameters: []*schema.UIParameter{
				{Label: "T5S1"},
				{Label: "T5S2"},
			},
			Sort: 100,
		}, {
			Label: "P6",
			Validate: &schema.Validate{
				Required: true,
			},
			SubParameters: []*schema.UIParameter{
				{Label: "P6S1"},
				{Label: "P6S2"},
				{Label: "P6S3"},
			},
			Sort: 100,
		},
	}

	var expectedParams = []*schema.UIParameter{
		{
			Label: "P1",
			Validate: &schema.Validate{
				Required: true,
			},
			SubParameters: []*schema.UIParameter{
				{Label: "P1S1"},
			},
			Sort: 100,
		}, {
			Label: "T5",
			Validate: &schema.Validate{
				Required: true,
			},
			SubParameters: []*schema.UIParameter{
				{Label: "T5S1"},
				{Label: "T5S2"},
			},
			Sort: 101,
		}, {
			Label: "P6",
			Validate: &schema.Validate{
				Required: true,
			},
			SubParameters: []*schema.UIParameter{
				{Label: "P6S1"},
				{Label: "P6S2"},
				{Label: "P6S3"},
			},
			Sort: 102,
		}, {
			Label: "T2",
			Validate: &schema.Validate{
				Required: true,
			},
			SubParameters: []*schema.UIParameter{
				{Label: "T2S1"},
				{Label: "T2S2"},
				{Label: "T2S3"},
			},
			Sort: 103,
		}, {
			Label: "P4",
			Validate: &schema.Validate{
				Required: false,
			},
			Sort: 104,
		}, {
			Label: "T3",
			Validate: &schema.Validate{
				Required: false,
			},
			Sort: 105,
		},
	}

	sortDefaultUISchema(params)
	for i, param := range params {
		Expect(param.Label).Should(Equal(expectedParams[i].Label))
		Expect(param.Sort).Should(Equal(expectedParams[i].Sort))
	}
}

func TestDefinitionQueryOption(t *testing.T) {
	assert.Equal(t, DefinitionQueryOption{
		Type: "workflowstep",
	}.String() == DefinitionQueryOption{
		Type:     "workflowstep",
		QueryAll: true,
	}.String(), false)
}
