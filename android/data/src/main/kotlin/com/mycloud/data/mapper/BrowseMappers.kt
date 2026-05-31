package com.mycloud.data.mapper

import com.mycloud.core.database.entity.FileEntity
import com.mycloud.core.database.entity.FolderEntity
import com.mycloud.core.model.Breadcrumb
import com.mycloud.core.model.FileNode
import com.mycloud.core.model.Folder
import com.mycloud.core.model.Permission
import com.mycloud.core.model.StorageStats
import com.mycloud.core.network.dto.BreadcrumbDto
import com.mycloud.core.network.dto.FileDto
import com.mycloud.core.network.dto.FolderDto
import com.mycloud.core.network.dto.StorageStatsDto
import java.time.Instant

fun FileDto.toEntity(sortIndex: Int, cachedAtMillis: Long): FileEntity = FileEntity(
    id = id,
    name = name,
    sizeBytes = sizeBytes,
    mimeType = mimeType,
    folderId = folderId,
    isStarred = isStarred,
    createdAtMillis = createdAt.isoToEpochMillis(),
    updatedAtMillis = updatedAt.isoToEpochMillis(),
    ownerId = ownerId,
    permission = permission,
    shared = shared,
    sortIndex = sortIndex,
    cachedAtMillis = cachedAtMillis,
)

fun FileEntity.toDomain(): FileNode = FileNode(
    id = id,
    name = name,
    sizeBytes = sizeBytes,
    mimeType = mimeType,
    folderId = folderId,
    isStarred = isStarred,
    createdAtMillis = createdAtMillis,
    updatedAtMillis = updatedAtMillis,
    ownerId = ownerId,
    permission = permission.toPermission(),
    shared = shared,
)

fun FolderDto.toEntity(): FolderEntity = FolderEntity(
    id = id,
    name = name,
    parentId = parentId,
    createdAtMillis = createdAt.isoToEpochMillis(),
    updatedAtMillis = updatedAt.isoToEpochMillis(),
    isFavorited = isFavorited,
    ownerId = ownerId,
    permission = permission,
    shared = shared,
)

fun FolderEntity.toDomain(): Folder = Folder(
    id = id,
    name = name,
    parentId = parentId,
    createdAtMillis = createdAtMillis,
    updatedAtMillis = updatedAtMillis,
    isFavorited = isFavorited,
    ownerId = ownerId,
    permission = permission.toPermission(),
    shared = shared,
)

fun BreadcrumbDto.toDomain(): Breadcrumb = Breadcrumb(id = id, name = name)

fun StorageStatsDto.toDomain(): StorageStats = StorageStats(
    quotaBytes = quotaBytes,
    usedBytes = usedBytes,
    availableBytes = availableBytes,
    fileCount = fileCount,
    folderCount = folderCount,
)

private fun String?.toPermission(): Permission? = when (this?.lowercase()) {
    "read" -> Permission.READ
    "write" -> Permission.WRITE
    "owner" -> Permission.OWNER
    else -> null
}

internal fun String?.isoToEpochMillis(): Long =
    this?.let { runCatching { Instant.parse(it).toEpochMilli() }.getOrNull() } ?: 0L
