package com.mycloud.core.network.dto

import kotlinx.serialization.Serializable

// NOTE: id defaults to "" because POST /shares replies with only {token, url}
// (no id) — without a default, that response fails to deserialize and surfaces
// as a misleading "can't reach the server" error.
@Serializable
data class ShareDto(
    val id: String = "",
    val token: String,
    val permission: String = "read",
    val createdAt: String? = null,
    val downloadCount: Long = 0,
    val expiresAt: String? = null,
    val downloadLimit: Long? = null,
    val fileId: String? = null,
    val fileName: String? = null,
    val folderId: String? = null,
)

@Serializable
data class SharesResponse(
    val shares: List<ShareDto> = emptyList(),
)

@Serializable
data class CreateShareRequestDto(
    val fileId: String? = null,
    val folderId: String? = null,
    val permission: String = "read",
    val password: String? = null,
    val expiresAt: String? = null,
    val downloadLimit: Long? = null,
    val singleView: Boolean = false,
)

// Field names mirror the backend's grant payload exactly:
// {id, permission, created_at, grantee_id, granted_by, grantee_name, granter_name,
//  file_id, file_name, folder_id, folder_name}. All optional/defaulted so both the
// list response and the leaner create response ({id, permission, grantee_id}) decode.
@Serializable
data class GrantDto(
    val id: String = "",
    val granteeId: String? = null,
    val granteeName: String? = null,
    val granterName: String? = null,
    val permission: String = "viewer",
    val fileId: String? = null,
    val fileName: String? = null,
    val folderId: String? = null,
    val folderName: String? = null,
    val createdAt: String? = null,
)

@Serializable
data class GrantsResponse(
    val grants: List<GrantDto> = emptyList(),
)

// The backend resolves `grantee` by username OR email, and grant permissions are
// viewer | editor | owner (not the share link's read/write).
@Serializable
data class CreateGrantRequestDto(
    val grantee: String,
    val fileId: String? = null,
    val folderId: String? = null,
    val permission: String = "viewer",
)
