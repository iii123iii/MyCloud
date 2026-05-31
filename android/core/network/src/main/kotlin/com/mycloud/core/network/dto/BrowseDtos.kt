package com.mycloud.core.network.dto

import kotlinx.serialization.Serializable

// Field names are camelCase; JsonNamingStrategy.SnakeCase maps to the backend.

@Serializable
data class FileDto(
    val id: String,
    val name: String,
    val sizeBytes: Long = 0,
    val mimeType: String? = null,
    val folderId: String? = null,
    val isStarred: Boolean = false,
    val createdAt: String? = null,
    val updatedAt: String? = null,
    val ownerId: String? = null,
    val permission: String? = null,
    val shared: Boolean = false,
)

/** `data` payload of GET /files. The cursor is in the envelope `meta`. */
@Serializable
data class FilesResponse(
    val files: List<FileDto> = emptyList(),
)

@Serializable
data class FolderDto(
    val id: String,
    val name: String,
    val parentId: String? = null,
    val createdAt: String? = null,
    val updatedAt: String? = null,
    val isFavorited: Boolean = false,
    val ownerId: String? = null,
    val permission: String? = null,
    val shared: Boolean = false,
)

@Serializable
data class FoldersResponse(
    val folders: List<FolderDto> = emptyList(),
)

@Serializable
data class BreadcrumbDto(
    val id: String,
    val name: String,
)

@Serializable
data class FolderPathResponse(
    val path: List<BreadcrumbDto> = emptyList(),
)

@Serializable
data class CreateFolderRequestDto(
    val name: String,
    val parentId: String? = null,
)

/** PATCH /files/{id}: any subset (rename / star / move). */
@Serializable
data class UpdateFileRequestDto(
    val name: String? = null,
    val isStarred: Boolean? = null,
    val folderId: String? = null,
)

/** PATCH /folders/{id}: rename and/or move. */
@Serializable
data class UpdateFolderRequestDto(
    val name: String? = null,
    val parentId: String? = null,
)

@Serializable
data class StorageStatsDto(
    val quotaBytes: Long = 0,
    val usedBytes: Long = 0,
    val availableBytes: Long = 0,
    val fileCount: Long = 0,
    val folderCount: Long = 0,
)
