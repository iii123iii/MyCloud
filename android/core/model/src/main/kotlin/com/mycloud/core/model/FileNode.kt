package com.mycloud.core.model

/** A file, from `GET /files`. Timestamps are epoch milliseconds. */
data class FileNode(
    val id: String,
    val name: String,
    val sizeBytes: Long,
    val mimeType: String?,
    val folderId: String?,
    val isStarred: Boolean,
    val createdAtMillis: Long,
    val updatedAtMillis: Long,
    // Present only when the file is reached via a share/grant.
    val ownerId: String? = null,
    val permission: Permission? = null,
    val shared: Boolean = false,
)

/** A folder, from `GET /folders`. */
data class Folder(
    val id: String,
    val name: String,
    val parentId: String?,
    val createdAtMillis: Long,
    val updatedAtMillis: Long,
    val isFavorited: Boolean = false,
    val ownerId: String? = null,
    val permission: Permission? = null,
    val shared: Boolean = false,
)

/** One hop of a folder path, from `GET /folders/{id}/path`. */
data class Breadcrumb(
    val id: String,
    val name: String,
)
