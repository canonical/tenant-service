// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package grpcutil

import (
	"fmt"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// ReadOnlyMethods returns a set of gRPC full method names that map to HTTP GET,
// indicating they are read-only and should not be wrapped in a transaction.
// The method names are in the format "/package.ServiceName/MethodName".
// Returns an error if the proto descriptor cannot be resolved from the registry.
func ReadOnlyMethods(sd grpc.ServiceDesc) (map[string]bool, error) {
	result := make(map[string]bool)

	meta, ok := sd.Metadata.(string)
	if !ok {
		return result, fmt.Errorf("service %q metadata is not a string: got %T", sd.ServiceName, sd.Metadata)
	}

	fileDesc, err := protoregistry.GlobalFiles.FindFileByPath(meta)
	if err != nil {
		return result, fmt.Errorf("failed to find proto file for service %q: %w", sd.ServiceName, err)
	}

	services := fileDesc.Services()
	for i := range services.Len() {
		svc := services.Get(i)
		if string(svc.FullName()) != sd.ServiceName {
			continue
		}

		methods := svc.Methods()
		for j := range methods.Len() {
			method := methods.Get(j)
			if extractHTTPVerb(method) == "GET" {
				fullMethod := "/" + sd.ServiceName + "/" + string(method.Name())
				result[fullMethod] = true
			}
		}
		break
	}

	return result, nil
}

// extractHTTPVerb returns the HTTP verb (GET, POST, PATCH, DELETE, PUT) from the
// google.api.http annotation on a method, or an empty string if not annotated.
func extractHTTPVerb(method protoreflect.MethodDescriptor) string {
	opts := method.Options()
	if opts == nil {
		return ""
	}

	httpRule, ok := proto.GetExtension(opts, annotations.E_Http).(*annotations.HttpRule)
	if !ok || httpRule == nil {
		return ""
	}

	switch httpRule.GetPattern().(type) {
	case *annotations.HttpRule_Get:
		return "GET"
	case *annotations.HttpRule_Post:
		return "POST"
	case *annotations.HttpRule_Patch:
		return "PATCH"
	case *annotations.HttpRule_Delete:
		return "DELETE"
	case *annotations.HttpRule_Put:
		return "PUT"
	default:
		return ""
	}
}
