package com.mycloud.core.model

enum class TransferDirection { UPLOAD, DOWNLOAD }

enum class TransferStatus { QUEUED, RUNNING, SUCCEEDED, FAILED }

/** A single in-flight or finished upload/download, surfaced to the Transfers UI. */
data class Transfer(
    val id: String,
    val name: String,
    val direction: TransferDirection,
    val status: TransferStatus,
    /** 0..100; best-effort (0 until the worker starts reporting). */
    val progress: Int,
)
