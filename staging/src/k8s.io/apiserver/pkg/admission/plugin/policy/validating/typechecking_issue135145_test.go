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

package validating

import (
	"testing"

	"k8s.io/apiserver/pkg/cel/common"
	"k8s.io/apiserver/pkg/cel/openapi"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

// TestIssue135145_SchemaDeclTypeReturnsNil is a reproducer for
// https://github.com/kubernetes/kubernetes/issues/135145
//
// This test validates the core claim that common.SchemaDeclType returns nil when given
// a schema with type=object and additionalProperties=true (with no schema).
func TestIssue135145_SchemaDeclTypeReturnsNil(t *testing.T) {
	tests := []struct {
		name        string
		schema      *spec.Schema
		expectNil   bool
		description string
	}{
		{
			name: "object with additionalProperties true and nil schema",
			schema: &spec.Schema{
				SchemaProps: spec.SchemaProps{
					Type: []string{"object"},
					AdditionalProperties: &spec.SchemaOrBool{
						Allows: true,
						Schema: nil, // No schema specified
					},
				},
			},
			expectNil:   true,
			description: "CORE CLAIM: type=object with additionalProperties=true (no schema) should return nil",
		},
		{
			name: "object with additionalProperties true and empty schema",
			schema: &spec.Schema{
				SchemaProps: spec.SchemaProps{
					Type: []string{"object"},
					AdditionalProperties: &spec.SchemaOrBool{
						Allows: true,
						Schema: &spec.Schema{
							SchemaProps: spec.SchemaProps{
								// No type specified
							},
						},
					},
				},
			},
			expectNil:   true,
			description: "VARIANT: additionalProperties with untyped/empty schema should also return nil",
		},
		{
			name: "object with additionalProperties schema with type",
			schema: &spec.Schema{
				SchemaProps: spec.SchemaProps{
					Type: []string{"object"},
					AdditionalProperties: &spec.SchemaOrBool{
						Allows: true,
						Schema: &spec.Schema{
							SchemaProps: spec.SchemaProps{
								Type: []string{"string"},
							},
						},
					},
				},
			},
			expectNil:   false,
			description: "CONTROL: additionalProperties with proper schema should NOT return nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := common.SchemaDeclType(&openapi.Schema{Schema: tt.schema}, false)

			if tt.expectNil {
				if result == nil {
					t.Logf("✓ CLAIM VALIDATED: %s", tt.description)
					t.Logf("  SchemaDeclType correctly returned nil")
					t.Logf("  This nil value would cause a panic when .MaybeAssignTypeName() is called on it")
				} else {
					t.Errorf("✗ CLAIM NOT VALIDATED: %s", tt.description)
					t.Errorf("  Expected nil but got: %v", result)
				}
			} else {
				if result != nil {
					t.Logf("✓ CONTROL: %s", tt.description)
					t.Logf("  SchemaDeclType correctly returned non-nil: %v", result)
				} else {
					t.Errorf("✗ UNEXPECTED: %s", tt.description)
					t.Errorf("  Expected non-nil but got nil")
				}
			}
		})
	}
}

// TestIssue135145_MaybeAssignTypeNamePanic validates that calling MaybeAssignTypeName
// on a nil result causes a panic
func TestIssue135145_MaybeAssignTypeNamePanic(t *testing.T) {
	// Create the problematic schema
	schema := &spec.Schema{
		SchemaProps: spec.SchemaProps{
			Type: []string{"object"},
			AdditionalProperties: &spec.SchemaOrBool{
				Allows: true,
				Schema: nil,
			},
		},
	}

	// This mimics what happens in TypeChecker.declType()
	defer func() {
		if r := recover(); r != nil {
			t.Logf("✓ CLAIM VALIDATED: Calling .MaybeAssignTypeName() on nil result causes panic")
			t.Logf("  Panic value: %v", r)
		} else {
			t.Error("✗ CLAIM NOT VALIDATED: No panic occurred")
		}
	}()

	// This should panic
	result := common.SchemaDeclType(&openapi.Schema{Schema: schema}, false)
	// The following line should cause a nil pointer dereference
	_ = result.MaybeAssignTypeName("TestType")

	t.Error("Should not reach here - expected panic")
}
