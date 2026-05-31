package com.mycloud.core.model

/** A public share link (GET/POST /shares). */
data class Share(
    val id: String,
    val token: String,
    val permission: Permission,
    val fileId: String?,
    val fileName: String?,
    val folderId: String?,
    val downloadCount: Long,
    val downloadLimit: Long?,
    val createdAtMillis: Long,
    val expiresAtMillis: Long?,
)

/** A per-user access grant (GET/POST /shares/grants). */
data class Grant(
    val id: String,
    val granteeUserId: String,
    val granteeName: String?,
    val permission: Permission,
    val fileId: String?,
    val folderId: String?,
    val createdAtMillis: Long,
)
