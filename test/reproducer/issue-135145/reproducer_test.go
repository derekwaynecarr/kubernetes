/*
Copyright 2025 The Kubernetes Authors.

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

// Reproducer for https://github.com/kubernetes/kubernetes/issues/135145
// This test validates the claims in the issue about ValidatingAdmissionPolicy
// panicking when encountering a CRD with type=object and additionalProperties=true

package reproducer_test

import (
	"testing"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	validatingadmissionpolicy "k8s.io/apiserver/pkg/admission/plugin/policy/validating"
	"k8s.io/apiserver/pkg/cel/openapi/resolver"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

// TestIssue135145_AdditionalPropertiesTrue validates that type-checking a ValidatingAdmissionPolicy
// that references a CRD with type=object and additionalProperties=true does not panic.
func TestIssue135145_AdditionalPropertiesTrue(t *testing.T) {
	// Create a schema resolver that returns a schema similar to the one in the issue
	// The problematic schema has:
	//   type: object
	//   properties:
	//     problematicProperty:
	//       type: object
	//       additionalProperties: true

	schemaMap := map[schema.GroupVersionKind]*spec.Schema{
		{Group: "example.com", Version: "v1alpha1", Kind: "Reproducer"}: {
			SchemaProps: spec.SchemaProps{
				Type: []string{"object"},
				Properties: map[string]spec.Schema{
					"apiVersion": {
						SchemaProps: spec.SchemaProps{
							Type: []string{"string"},
						},
					},
					"kind": {
						SchemaProps: spec.SchemaProps{
							Type: []string{"string"},
						},
					},
					"metadata": {
						SchemaProps: spec.SchemaProps{
							Type: []string{"object"},
						},
					},
					"spec": {
						SchemaProps: spec.SchemaProps{
							Type: []string{"object"},
							Properties: map[string]spec.Schema{
								"text": {
									SchemaProps: spec.SchemaProps{
										Type:        []string{"string"},
										Description: "Some text content.",
									},
								},
							},
						},
					},
					"status": {
						SchemaProps: spec.SchemaProps{
							Type:        []string{"object"},
							Description: "ReproducerStatus defines the observed state of Reproducer.",
							Properties: map[string]spec.Schema{
								"problematicProperty": {
									SchemaProps: spec.SchemaProps{
										Type:        []string{"object"},
										Description: "Adding this property makes the controller panic",
										// This is the key: additionalProperties: true with no schema
										AdditionalProperties: &spec.SchemaOrBool{
											Allows: true,
											Schema: nil, // No schema specified, just allows: true
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Create a mock resolver
	mockResolver := &mockSchemaResolver{schemas: schemaMap}

	// Create the TypeChecker
	typeChecker := &validatingadmissionpolicy.TypeChecker{
		SchemaResolver: mockResolver,
		RestMapper:     newMockRESTMapper(),
	}

	// Create a ValidatingAdmissionPolicy that references the CRD
	policy := &admissionregistrationv1.ValidatingAdmissionPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "reproducer-validation",
		},
		Spec: admissionregistrationv1.ValidatingAdmissionPolicySpec{
			FailurePolicy: func() *admissionregistrationv1.FailurePolicyType {
				fp := admissionregistrationv1.Fail
				return &fp
			}(),
			MatchConstraints: &admissionregistrationv1.MatchResources{
				ResourceRules: []admissionregistrationv1.NamedRuleWithOperations{
					{
						RuleWithOperations: admissionregistrationv1.RuleWithOperations{
							Operations: []admissionregistrationv1.OperationType{"CREATE", "UPDATE"},
							Rule: admissionregistrationv1.Rule{
								APIGroups:   []string{"example.com"},
								APIVersions: []string{"v1alpha1"},
								Resources:   []string{"reproducers"},
							},
						},
					},
				},
			},
			Validations: []admissionregistrationv1.Validation{
				{
					Expression: "has(object.spec.text)",
					Reason:     func() *admissionregistrationv1.ReasonType { r := admissionregistrationv1.ReasonForbidden; return &r }(),
					Message:    "Validation failed",
				},
			},
		},
	}

	// This should not panic according to expected behavior
	// But based on the issue, it currently does panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("CLAIM VALIDATED: TypeChecker.Check() panicked when processing a CRD with type=object and additionalProperties=true: %v", r)
		} else {
			t.Log("CLAIM NOT VALIDATED: TypeChecker.Check() did not panic (this might mean the issue is fixed or the reproducer is incomplete)")
		}
	}()

	warnings := typeChecker.Check(policy)
	t.Logf("Type checking completed with %d warnings", len(warnings))
	for i, w := range warnings {
		t.Logf("  Warning %d: %s - %s", i, w.FieldRef, w.Warning)
	}
}

// mockSchemaResolver implements resolver.SchemaResolver for testing
type mockSchemaResolver struct {
	schemas map[schema.GroupVersionKind]*spec.Schema
}

func (m *mockSchemaResolver) ResolveSchema(gvk schema.GroupVersionKind) (*spec.Schema, error) {
	if s, ok := m.schemas[gvk]; ok {
		return s, nil
	}
	return nil, resolver.ErrSchemaNotFound
}

// mockRESTMapper is a simple mock for meta.RESTMapper
type mockRESTMapper struct{}

func newMockRESTMapper() *mockRESTMapper {
	return &mockRESTMapper{}
}

func (m *mockRESTMapper) KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	// Map reproducers resource to Reproducer kind
	if resource.Group == "example.com" && resource.Version == "v1alpha1" && resource.Resource == "reproducers" {
		return []schema.GroupVersionKind{
			{Group: "example.com", Version: "v1alpha1", Kind: "Reproducer"},
		}, nil
	}
	return nil, &mockNoMatchError{}
}

func (m *mockRESTMapper) ResourceFor(input schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	return schema.GroupVersionResource{}, &mockNoMatchError{}
}

func (m *mockRESTMapper) ResourcesFor(input schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	return nil, &mockNoMatchError{}
}

func (m *mockRESTMapper) KindFor(resource schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, &mockNoMatchError{}
}

func (m *mockRESTMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*RESTMapping, error) {
	return nil, &mockNoMatchError{}
}

func (m *mockRESTMapper) RESTMappings(gk schema.GroupKind, versions ...string) ([]*RESTMapping, error) {
	return nil, &mockNoMatchError{}
}

func (m *mockRESTMapper) ResourceSingularizer(resource string) (singular string, err error) {
	return resource, nil
}

func (m *mockRESTMapper) Reset() {}

// RESTMapping is a simplified version for the mock
type RESTMapping struct{}

// mockNoMatchError implements error
type mockNoMatchError struct{}

func (e *mockNoMatchError) Error() string {
	return "no matches found"
}
