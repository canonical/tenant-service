// Copyright 2025 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package authorization

const (
	OWNER_RELATION  = "owner"
	MEMBER_RELATION = "member"

	PRIVILEGED_RELATION = "privileged"
	ADMIN_RELATION      = "admin"

	CAN_VIEW_PERMISSION   = "can_view"
	CAN_EDIT_PERMISSION   = "can_edit"
	CAN_CREATE_PERMISSION = "can_create"
	CAN_DELETE_PERMISSION = "can_delete"
)

func UserTuple(userId string) string {
	return "user:" + userId
}

func TenantTuple(tenantId string) string {
	return "tenant:" + tenantId
}

func PrivilegedTuple(privilegedId string) string {
	return "privileged:" + privilegedId
}
