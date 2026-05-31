package com.mycloud.core.database.security

import android.content.Context
import android.util.Base64
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import com.google.crypto.tink.Aead
import com.google.crypto.tink.KeyTemplates
import com.google.crypto.tink.aead.AeadConfig
import com.google.crypto.tink.integration.android.AndroidKeysetManager
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import java.security.SecureRandom
import javax.inject.Inject
import javax.inject.Singleton

private val Context.dbKeyDataStore by preferencesDataStore(name = "secure_db_key")

/**
 * Supplies the SQLCipher passphrase for the Room cache. A random 32-byte key is
 * generated once, sealed with a Tink AEAD whose keyset is wrapped by an Android
 * Keystore master key, and persisted (ciphertext) in a Preferences DataStore — so
 * the on-disk DB key is never stored in the clear.
 *
 * The read is synchronous because Room opens the database lazily on a background
 * thread when the first query runs; it happens once for the process-lifetime
 * singleton DB. Mirrors the crypto setup in `:core:datastore`'s SecureTokenStore
 * but is kept self-contained here so `:core:database` needs no extra module dep.
 */
@Singleton
class DbKeyProvider @Inject constructor(
    @ApplicationContext private val context: Context,
) {
    private val keyPref = stringPreferencesKey("db_passphrase")
    private val aead: Aead by lazy { buildAead(context) }

    /**
     * The raw passphrase bytes. A fresh copy is returned each call because
     * SQLCipher's SupportFactory zeroes the array it is handed after opening.
     */
    fun passphrase(): ByteArray = runBlocking {
        val existing = runCatching { context.dbKeyDataStore.data.first()[keyPref] }
            .getOrNull()
            ?.let(::decode)
        (existing ?: generateAndStore()).copyOf()
    }

    private suspend fun generateAndStore(): ByteArray {
        val key = ByteArray(32).also { SecureRandom().nextBytes(it) }
        val ciphertext = aead.encrypt(key, ASSOCIATED_DATA)
        val encoded = Base64.encodeToString(ciphertext, Base64.NO_WRAP)
        context.dbKeyDataStore.edit { it[keyPref] = encoded }
        return key
    }

    private fun decode(encoded: String): ByteArray? = runCatching {
        aead.decrypt(Base64.decode(encoded, Base64.NO_WRAP), ASSOCIATED_DATA)
    }.getOrNull()

    private companion object {
        val ASSOCIATED_DATA = "mycloud_db_key".toByteArray()

        fun buildAead(context: Context): Aead {
            AeadConfig.register()
            val handle = AndroidKeysetManager.Builder()
                .withSharedPref(context, "mycloud_dbkey_keyset", "mycloud_dbkey_prefs")
                .withKeyTemplate(KeyTemplates.get("AES256_GCM"))
                .withMasterKeyUri("android-keystore://mycloud_dbkey_master_key")
                .build()
                .keysetHandle
            return handle.getPrimitive(Aead::class.java)
        }
    }
}
