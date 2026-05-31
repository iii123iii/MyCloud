package com.mycloud.core.network.dto

import kotlinx.serialization.Serializable

@Serializable
data class CommentDto(
    val id: String,
    val userId: String? = null,
    val username: String? = null,
    val body: String = "",
    val createdAt: String? = null,
    /** Server-computed: true when the caller authored this comment. */
    val editable: Boolean = false,
)

@Serializable
data class CommentsResponse(
    val comments: List<CommentDto> = emptyList(),
)

@Serializable
data class CreateCommentRequestDto(
    val body: String,
)

@Serializable
data class FileVersionDto(
    val versionNo: Int = 0,
    val sizeBytes: Long = 0,
    val createdAt: String? = null,
)

@Serializable
data class VersionsResponse(
    val versions: List<FileVersionDto> = emptyList(),
)
