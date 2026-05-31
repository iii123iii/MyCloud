package com.mycloud.data.mapper

import com.mycloud.core.model.Comment
import com.mycloud.core.model.FileVersion
import com.mycloud.core.model.Photo
import com.mycloud.core.network.dto.CommentDto
import com.mycloud.core.network.dto.FileVersionDto
import com.mycloud.core.network.dto.PhotoDto

fun PhotoDto.toDomain(): Photo = Photo(
    id = id,
    name = name,
    sizeBytes = sizeBytes,
    mimeType = mimeType,
    width = width,
    height = height,
    takenAtMillis = (shotAt ?: createdAt).isoToEpochMillis(),
)

fun CommentDto.toDomain(): Comment = Comment(
    id = id,
    authorName = username,
    content = body,
    createdAtMillis = createdAt.isoToEpochMillis(),
    editable = editable,
)

fun FileVersionDto.toDomain(): FileVersion = FileVersion(
    versionNumber = versionNo,
    sizeBytes = sizeBytes,
    createdAtMillis = createdAt.isoToEpochMillis(),
)
