package com.mycloud.data.mapper

import androidx.work.WorkInfo
import com.mycloud.core.model.Transfer
import com.mycloud.core.model.TransferDirection
import com.mycloud.core.model.TransferStatus
import com.mycloud.core.work.TransferKeys

fun WorkInfo.toTransfer(): Transfer {
    val direction = if (tags.contains(TransferKeys.TAG_UPLOAD)) {
        TransferDirection.UPLOAD
    } else {
        TransferDirection.DOWNLOAD
    }
    val name = tags.firstOrNull { it.startsWith(TransferKeys.NAME_TAG_PREFIX) }
        ?.removePrefix(TransferKeys.NAME_TAG_PREFIX)
        ?: "file"
    val status = when (state) {
        WorkInfo.State.ENQUEUED, WorkInfo.State.BLOCKED -> TransferStatus.QUEUED
        WorkInfo.State.RUNNING -> TransferStatus.RUNNING
        WorkInfo.State.SUCCEEDED -> TransferStatus.SUCCEEDED
        WorkInfo.State.FAILED, WorkInfo.State.CANCELLED -> TransferStatus.FAILED
    }
    val progress = progress.getInt(TransferKeys.KEY_PROGRESS, 0)
    return Transfer(
        id = id.toString(),
        name = name,
        direction = direction,
        status = status,
        progress = progress,
    )
}
