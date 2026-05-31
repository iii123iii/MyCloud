package com.mycloud.core.model

/** A photo in the timeline (GET /photos). */
data class Photo(
    val id: String,
    val name: String,
    val sizeBytes: Long,
    val mimeType: String?,
    val width: Int?,
    val height: Int?,
    /** When the photo was taken (falls back to created time when unknown). */
    val takenAtMillis: Long,
)
