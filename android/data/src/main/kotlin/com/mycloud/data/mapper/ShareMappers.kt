package com.mycloud.data.mapper

import com.mycloud.core.model.Grant
import com.mycloud.core.model.Permission
import com.mycloud.core.model.Share
import com.mycloud.core.network.dto.GrantDto
import com.mycloud.core.network.dto.ShareDto

fun ShareDto.toDomain(): Share = Share(
    id = id,
    token = token,
    permission = permission.toPermissionOrRead(),
    fileId = fileId,
    fileName = fileName,
    folderId = folderId,
    downloadCount = downloadCount,
    downloadLimit = downloadLimit,
    createdAtMillis = createdAt.isoToEpochMillis(),
    expiresAtMillis = expiresAt?.isoToEpochMillis()?.takeIf { it != 0L },
)

fun GrantDto.toDomain(): Grant = Grant(
    id = id,
    granteeUserId = granteeId.orEmpty(),
    granteeName = granteeName,
    permission = permission.toPermissionOrRead(),
    fileId = fileId,
    folderId = folderId,
    createdAtMillis = createdAt.isoToEpochMillis(),
)

private fun String.toPermissionOrRead(): Permission = when (lowercase()) {
    // Share links use read/write; per-user grants use viewer/editor/owner.
    "write", "editor" -> Permission.WRITE
    "owner" -> Permission.OWNER
    else -> Permission.READ
}
