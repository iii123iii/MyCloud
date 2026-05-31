package com.mycloud.core.network.dto

import kotlinx.serialization.Serializable

@Serializable
data class PhotoDto(
    val id: String,
    val name: String,
    val sizeBytes: Long = 0,
    val mimeType: String? = null,
    val width: Int? = null,
    val height: Int? = null,
    val shotAt: String? = null,
    val createdAt: String? = null,
)

@Serializable
data class PhotosResponse(
    val photos: List<PhotoDto> = emptyList(),
)
