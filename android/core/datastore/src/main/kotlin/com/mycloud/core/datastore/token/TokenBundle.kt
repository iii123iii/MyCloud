package com.mycloud.core.datastore.token

import kotlinx.serialization.Serializable

/** The persisted session secret set. Encrypted at rest via Tink/Keystore. */
@Serializable
data class TokenBundle(
    val accessToken: String,
    val accessExpiresAtMillis: Long,
    val refreshToken: String,
    /** Token family id — refresh rotates within it; reuse revokes the family. */
    val family: String,
    val userId: String,
)
