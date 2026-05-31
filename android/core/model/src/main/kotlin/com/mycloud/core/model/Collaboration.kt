package com.mycloud.core.model

/** A comment on a file (GET/POST /files/{id}/comments). */
data class Comment(
    val id: String,
    val authorName: String?,
    val content: String,
    val createdAtMillis: Long,
    /** True when the current user authored this comment (may edit/delete it). */
    val editable: Boolean = false,
)

/** A stored revision of a file (GET /files/{id}/versions). */
data class FileVersion(
    val versionNumber: Int,
    val sizeBytes: Long,
    val createdAtMillis: Long,
)
