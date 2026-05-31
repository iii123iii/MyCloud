package com.mycloud.core.network.dto

import kotlinx.serialization.Serializable

@Serializable
data class TrashItemDto(
    val type: String = "file", // "file" | "folder"
    val id: String,
    val name: String,
    val sizeBytes: Long = 0,
    val mimeType: String? = null,
    val deletedAt: String? = null,
)

@Serializable
data class TrashResponse(
    val items: List<TrashItemDto> = emptyList(),
)
