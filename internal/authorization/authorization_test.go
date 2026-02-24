// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package authorization

import (
	"context"
	"errors"
	"testing"

	fga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/mock/gomock"

	"github.com/canonical/tenant-service/internal/openfga"
)

//go:generate mockgen -build_flags=--mod=mod -package authorization -destination ./mock_interfaces.go -source=./interfaces.go
//go:generate mockgen -build_flags=--mod=mod -package authorization -destination ./mock_logger.go -source=../logging/interfaces.go
//go:generate mockgen -build_flags=--mod=mod -package authorization -destination ./mock_monitor.go -source=../monitoring/interfaces.go
//go:generate mockgen -build_flags=--mod=mod -package authorization -destination ./mock_tracing.go -source=../tracing/interfaces.go

func TestAuthorizer_Check(t *testing.T) {
	user := "user:123"
	relation := "member"
	object := "tenant:456"
	contextualTuples := []openfga.Tuple{*openfga.NewTuple("user:789", "owner", "tenant:456")}

	testCases := []struct {
		name           string
		setupMocks     func(*MockAuthzClientInterface)
		expectedResult bool
		expectedErr    bool
	}{
		{
			name: "success - allowed",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().Check(gomock.Any(), user, relation, object, contextualTuples).Return(true, nil)
			},
			expectedResult: true,
			expectedErr:    false,
		},
		{
			name: "success - not allowed",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().Check(gomock.Any(), user, relation, object, contextualTuples).Return(false, nil)
			},
			expectedResult: false,
			expectedErr:    false,
		},
		{
			name: "error - client error",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().Check(gomock.Any(), user, relation, object, contextualTuples).Return(false, errors.New("client error"))
			},
			expectedResult: false,
			expectedErr:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := NewMockAuthzClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockMonitor := NewMockMonitorInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)

			a := NewAuthorizer(mockClient, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "authorization.Authorizer.Check").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockClient)

			result, err := a.Check(context.Background(), user, relation, object, contextualTuples...)

			if tc.expectedErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if result != tc.expectedResult {
				t.Errorf("expected result %v, got %v", tc.expectedResult, result)
			}
		})
	}
}

func TestAuthorizer_ListObjects(t *testing.T) {
	user := "user:123"
	relation := "member"
	objectType := "tenant"
	objects := []string{"tenant:1", "tenant:2", "tenant:3"}

	testCases := []struct {
		name        string
		setupMocks  func(*MockAuthzClientInterface)
		expectedErr bool
	}{
		{
			name: "success",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().ListObjects(gomock.Any(), user, relation, objectType).Return(objects, nil)
			},
			expectedErr: false,
		},
		{
			name: "error - client error",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().ListObjects(gomock.Any(), user, relation, objectType).Return(nil, errors.New("client error"))
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := NewMockAuthzClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockMonitor := NewMockMonitorInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)

			a := NewAuthorizer(mockClient, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "authorization.Authorizer.ListObjects").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockClient)

			result, err := a.ListObjects(context.Background(), user, relation, objectType)

			if tc.expectedErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if len(result) != len(objects) {
					t.Errorf("expected %d objects, got %d", len(objects), len(result))
				}
			}
		})
	}
}

func TestAuthorizer_FilterObjects(t *testing.T) {
	user := "user:123"
	relation := "member"
	objectType := "tenant"
	requestedObjs := []string{"tenant:1", "tenant:2", "tenant:3", "tenant:4"}
	allowedObjs := []string{"tenant:1", "tenant:3", "tenant:5"}

	testCases := []struct {
		name           string
		setupMocks     func(*MockAuthzClientInterface)
		expectedResult []string
		expectedErr    bool
	}{
		{
			name: "success - filters correctly",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().ListObjects(gomock.Any(), user, relation, objectType).Return(allowedObjs, nil)
			},
			expectedResult: []string{"tenant:1", "tenant:3"},
			expectedErr:    false,
		},
		{
			name: "success - no overlap",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().ListObjects(gomock.Any(), user, relation, objectType).Return([]string{"tenant:9"}, nil)
			},
			expectedResult: nil,
			expectedErr:    false,
		},
		{
			name: "error - list objects error",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().ListObjects(gomock.Any(), user, relation, objectType).Return(nil, errors.New("client error"))
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := NewMockAuthzClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockMonitor := NewMockMonitorInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)

			a := NewAuthorizer(mockClient, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "authorization.Authorizer.FilterObjects").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			mockTracer.EXPECT().Start(gomock.Any(), "authorization.Authorizer.ListObjects").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockClient)

			result, err := a.FilterObjects(context.Background(), user, relation, objectType, requestedObjs)

			if tc.expectedErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if len(result) != len(tc.expectedResult) {
					t.Errorf("expected %d filtered objects, got %d", len(tc.expectedResult), len(result))
				}
			}
		})
	}
}

func TestAuthorizer_ValidateModel(t *testing.T) {
	testCases := []struct {
		name        string
		setupMocks  func(*MockAuthzClientInterface)
		expectedErr error
	}{
		{
			name: "success - models match",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().CompareModel(gomock.Any(), gomock.Any()).Return(true, nil)
			},
			expectedErr: nil,
		},
		{
			name: "error - models do not match",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().CompareModel(gomock.Any(), gomock.Any()).Return(false, nil)
			},
			expectedErr: ErrInvalidAuthModel,
		},
		{
			name: "error - client error",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().CompareModel(gomock.Any(), gomock.Any()).Return(false, errors.New("client error"))
			},
			expectedErr: errors.New("client error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := NewMockAuthzClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockMonitor := NewMockMonitorInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)

			a := NewAuthorizer(mockClient, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "authorization.Authorizer.ValidateModel").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockClient)

			err := a.ValidateModel(context.Background())

			if tc.expectedErr != nil {
				if err == nil {
					t.Errorf("expected error %v but got none", tc.expectedErr)
				} else if tc.expectedErr == ErrInvalidAuthModel && err != ErrInvalidAuthModel {
					t.Errorf("expected ErrInvalidAuthModel but got %v", err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestAuthorizer_AssignTenantOwner(t *testing.T) {
	tenantID := "tenant-123"
	userID := "user-456"

	testCases := []struct {
		name        string
		setupMocks  func(*MockAuthzClientInterface)
		expectedErr bool
	}{
		{
			name: "success",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().WriteTuple(gomock.Any(), UserTuple(userID), OWNER_RELATION, TenantTuple(tenantID)).Return(nil)
			},
			expectedErr: false,
		},
		{
			name: "error - write tuple error",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().WriteTuple(gomock.Any(), UserTuple(userID), OWNER_RELATION, TenantTuple(tenantID)).Return(errors.New("write error"))
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := NewMockAuthzClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockMonitor := NewMockMonitorInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)

			a := NewAuthorizer(mockClient, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "authorization.Authorizer.AssignTenantOwner").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockClient)

			err := a.AssignTenantOwner(context.Background(), tenantID, userID)

			if tc.expectedErr && err == nil {
				t.Error("expected error but got none")
			} else if !tc.expectedErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestAuthorizer_AssignPrivilegedAdmin(t *testing.T) {
	privilegedID := "privileged-123"
	userID := "user-456"

	testCases := []struct {
		name        string
		setupMocks  func(*MockAuthzClientInterface)
		expectedErr bool
	}{
		{
			name: "success",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().WriteTuple(gomock.Any(), UserTuple(userID), ADMIN_RELATION, PrivilegedTuple(privilegedID)).Return(nil)
			},
			expectedErr: false,
		},
		{
			name: "error - write tuple error",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().WriteTuple(gomock.Any(), UserTuple(userID), ADMIN_RELATION, PrivilegedTuple(privilegedID)).Return(errors.New("write error"))
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := NewMockAuthzClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockMonitor := NewMockMonitorInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)

			a := NewAuthorizer(mockClient, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "authorization.Authorizer.AssignPrivilegedAdmin").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockClient)

			err := a.AssignPrivilegedAdmin(context.Background(), privilegedID, userID)

			if tc.expectedErr && err == nil {
				t.Error("expected error but got none")
			} else if !tc.expectedErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestAuthorizer_LinkTenantToPrivileged(t *testing.T) {
	tenantID := "tenant-123"
	privilegedID := "privileged-456"

	testCases := []struct {
		name        string
		setupMocks  func(*MockAuthzClientInterface)
		expectedErr bool
	}{
		{
			name: "success",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().WriteTuple(gomock.Any(), PrivilegedTuple(privilegedID), PRIVILEGED_RELATION, TenantTuple(tenantID)).Return(nil)
			},
			expectedErr: false,
		},
		{
			name: "error - write tuple error",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().WriteTuple(gomock.Any(), PrivilegedTuple(privilegedID), PRIVILEGED_RELATION, TenantTuple(tenantID)).Return(errors.New("write error"))
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := NewMockAuthzClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockMonitor := NewMockMonitorInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)

			a := NewAuthorizer(mockClient, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "authorization.Authorizer.LinkTenantToPrivileged").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockClient)

			err := a.LinkTenantToPrivileged(context.Background(), tenantID, privilegedID)

			if tc.expectedErr && err == nil {
				t.Error("expected error but got none")
			} else if !tc.expectedErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestAuthorizer_AssignTenantMember(t *testing.T) {
	tenantID := "tenant-123"
	userID := "user-456"

	testCases := []struct {
		name        string
		setupMocks  func(*MockAuthzClientInterface)
		expectedErr bool
	}{
		{
			name: "success",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().WriteTuple(gomock.Any(), UserTuple(userID), MEMBER_RELATION, TenantTuple(tenantID)).Return(nil)
			},
			expectedErr: false,
		},
		{
			name: "error - write tuple error",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().WriteTuple(gomock.Any(), UserTuple(userID), MEMBER_RELATION, TenantTuple(tenantID)).Return(errors.New("write error"))
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := NewMockAuthzClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockMonitor := NewMockMonitorInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)

			a := NewAuthorizer(mockClient, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "authorization.Authorizer.AssignTenantMember").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockClient)

			err := a.AssignTenantMember(context.Background(), tenantID, userID)

			if tc.expectedErr && err == nil {
				t.Error("expected error but got none")
			} else if !tc.expectedErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestAuthorizer_RemoveTenantOwner(t *testing.T) {
	tenantID := "tenant-123"
	userID := "user-456"

	testCases := []struct {
		name        string
		setupMocks  func(*MockAuthzClientInterface)
		expectedErr bool
	}{
		{
			name: "success",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().DeleteTuple(gomock.Any(), UserTuple(userID), OWNER_RELATION, TenantTuple(tenantID)).Return(nil)
			},
			expectedErr: false,
		},
		{
			name: "error - delete tuple error",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().DeleteTuple(gomock.Any(), UserTuple(userID), OWNER_RELATION, TenantTuple(tenantID)).Return(errors.New("delete error"))
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := NewMockAuthzClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockMonitor := NewMockMonitorInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)

			a := NewAuthorizer(mockClient, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "authorization.Authorizer.RemoveTenantOwner").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockClient)

			err := a.RemoveTenantOwner(context.Background(), tenantID, userID)

			if tc.expectedErr && err == nil {
				t.Error("expected error but got none")
			} else if !tc.expectedErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestAuthorizer_RemoveTenantMember(t *testing.T) {
	tenantID := "tenant-123"
	userID := "user-456"

	testCases := []struct {
		name        string
		setupMocks  func(*MockAuthzClientInterface)
		expectedErr bool
	}{
		{
			name: "success",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().DeleteTuple(gomock.Any(), UserTuple(userID), MEMBER_RELATION, TenantTuple(tenantID)).Return(nil)
			},
			expectedErr: false,
		},
		{
			name: "error - delete tuple error",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().DeleteTuple(gomock.Any(), UserTuple(userID), MEMBER_RELATION, TenantTuple(tenantID)).Return(errors.New("delete error"))
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := NewMockAuthzClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockMonitor := NewMockMonitorInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)

			a := NewAuthorizer(mockClient, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "authorization.Authorizer.RemoveTenantMember").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockClient)

			err := a.RemoveTenantMember(context.Background(), tenantID, userID)

			if tc.expectedErr && err == nil {
				t.Error("expected error but got none")
			} else if !tc.expectedErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestAuthorizer_CheckTenantAccess(t *testing.T) {
	tenantID := "tenant-123"
	userID := "user-456"
	relation := "member"

	testCases := []struct {
		name           string
		setupMocks     func(*MockAuthzClientInterface)
		expectedResult bool
		expectedErr    bool
	}{
		{
			name: "success - allowed",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().Check(gomock.Any(), UserTuple(userID), relation, TenantTuple(tenantID)).Return(true, nil)
			},
			expectedResult: true,
			expectedErr:    false,
		},
		{
			name: "success - not allowed",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().Check(gomock.Any(), UserTuple(userID), relation, TenantTuple(tenantID)).Return(false, nil)
			},
			expectedResult: false,
			expectedErr:    false,
		},
		{
			name: "error - check error",
			setupMocks: func(mockClient *MockAuthzClientInterface) {
				mockClient.EXPECT().Check(gomock.Any(), UserTuple(userID), relation, TenantTuple(tenantID)).Return(false, errors.New("check error"))
			},
			expectedResult: false,
			expectedErr:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := NewMockAuthzClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockMonitor := NewMockMonitorInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)

			a := NewAuthorizer(mockClient, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "authorization.Authorizer.CheckTenantAccess").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			mockTracer.EXPECT().Start(gomock.Any(), "authorization.Authorizer.Check").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockClient)

			result, err := a.CheckTenantAccess(context.Background(), tenantID, userID, relation)

			if tc.expectedErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if result != tc.expectedResult {
				t.Errorf("expected result %v, got %v", tc.expectedResult, result)
			}
		})
	}
}

func TestAuthorizer_DeleteTenant(t *testing.T) {
	tenantID := "tenant-123"

	testCases := []struct {
		name        string
		setupMocks  func(*MockAuthzClientInterface, *MockLoggerInterface)
		expectedErr bool
	}{
		{
			name: "success - single batch",
			setupMocks: func(mockClient *MockAuthzClientInterface, mockLogger *MockLoggerInterface) {
				tuples := []fga.Tuple{
					{Key: fga.TupleKey{User: "user:1", Relation: "owner", Object: TenantTuple(tenantID)}},
					{Key: fga.TupleKey{User: "user:2", Relation: "member", Object: TenantTuple(tenantID)}},
				}
				mockClient.EXPECT().ReadTuples(gomock.Any(), "", "", TenantTuple(tenantID), "").Return(&client.ClientReadResponse{
					Tuples:            tuples,
					ContinuationToken: "",
				}, nil)
				mockClient.EXPECT().DeleteTuples(gomock.Any(), gomock.Any()).Return(nil)
			},
			expectedErr: false,
		},
		{
			name: "success - multiple batches",
			setupMocks: func(mockClient *MockAuthzClientInterface, mockLogger *MockLoggerInterface) {
				batch1 := []fga.Tuple{
					{Key: fga.TupleKey{User: "user:1", Relation: "owner", Object: TenantTuple(tenantID)}},
				}
				batch2 := []fga.Tuple{
					{Key: fga.TupleKey{User: "user:2", Relation: "member", Object: TenantTuple(tenantID)}},
				}
				gomock.InOrder(
					mockClient.EXPECT().ReadTuples(gomock.Any(), "", "", TenantTuple(tenantID), "").Return(&client.ClientReadResponse{
						Tuples:            batch1,
						ContinuationToken: "token1",
					}, nil),
					mockClient.EXPECT().DeleteTuples(gomock.Any(), gomock.Any()).Return(nil),
					mockClient.EXPECT().ReadTuples(gomock.Any(), "", "", TenantTuple(tenantID), "token1").Return(&client.ClientReadResponse{
						Tuples:            batch2,
						ContinuationToken: "",
					}, nil),
					mockClient.EXPECT().DeleteTuples(gomock.Any(), gomock.Any()).Return(nil),
				)
			},
			expectedErr: false,
		},
		{
			name: "success - no tuples",
			setupMocks: func(mockClient *MockAuthzClientInterface, mockLogger *MockLoggerInterface) {
				mockClient.EXPECT().ReadTuples(gomock.Any(), "", "", TenantTuple(tenantID), "").Return(&client.ClientReadResponse{
					Tuples:            []fga.Tuple{},
					ContinuationToken: "",
				}, nil)
			},
			expectedErr: false,
		},
		{
			name: "error - read tuples error",
			setupMocks: func(mockClient *MockAuthzClientInterface, mockLogger *MockLoggerInterface) {
				mockClient.EXPECT().ReadTuples(gomock.Any(), "", "", TenantTuple(tenantID), "").Return(nil, errors.New("read error"))
				mockLogger.EXPECT().Errorf(gomock.Any(), gomock.Any())
			},
			expectedErr: true,
		},
		{
			name: "error - delete tuples error",
			setupMocks: func(mockClient *MockAuthzClientInterface, mockLogger *MockLoggerInterface) {
				tuples := []fga.Tuple{
					{Key: fga.TupleKey{User: "user:1", Relation: "owner", Object: TenantTuple(tenantID)}},
				}
				mockClient.EXPECT().ReadTuples(gomock.Any(), "", "", TenantTuple(tenantID), "").Return(&client.ClientReadResponse{
					Tuples:            tuples,
					ContinuationToken: "",
				}, nil)
				mockClient.EXPECT().DeleteTuples(gomock.Any(), gomock.Any()).Return(errors.New("delete error"))
				mockLogger.EXPECT().Errorf(gomock.Any(), gomock.Any(), gomock.Any())
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := NewMockAuthzClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockMonitor := NewMockMonitorInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)

			a := NewAuthorizer(mockClient, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "authorization.Authorizer.DeleteTenant").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockClient, mockLogger)

			err := a.DeleteTenant(context.Background(), tenantID)

			if tc.expectedErr && err == nil {
				t.Error("expected error but got none")
			} else if !tc.expectedErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
