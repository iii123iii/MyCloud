package com.mycloud.core.datastore.token

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
import kotlinx.serialization.json.Json
import java.util.concurrent.atomic.AtomicBoolean
import javax.inject.Inject
import javax.inject.Singleton

private val Context.tokenDataStore by preferencesDataStore(name = "secure_tokens")

/**
 * Encrypted token storage. The [TokenBundle] is serialized to JSON, sealed with
 * a Tink AEAD whose keyset is wrapped by an Android Keystore master key, and the
 * ciphertext is kept in a Preferences DataStore.
 *
 * An in-memory cache backs the synchronous reads the OkHttp auth layer needs
 * (interceptor/authenticator run on blocking dispatchers). Call [load] once at
 * startup; synchronous callers fall back to a one-time blocking read.
 */
@Singleton
class SecureTokenStore @Inject constructor(
    @ApplicationContext private val context: Context,
) {
    private val ciphertextKey = stringPreferencesKey("token_bundle")
    private val json = Json { ignoreUnknownKeys = true }
    private val aead: Aead by lazy { buildAead(context) }

    @Volatile
    private var cached: TokenBundle? = null
    private val loaded = AtomicBoolean(false)

    fun currentAccessToken(): String? = read()?.accessToken
    fun currentRefreshToken(): String? = read()?.refreshToken
    fun currentBundle(): TokenBundle? = read()
    fun hasSession(): Boolean = read() != null

    suspend fun save(bundle: TokenBundle) {
        cached = bundle
        loaded.set(true)
        val plaintext = json.encodeToString(TokenBundle.serializer(), bundle).toByteArray()
        val ciphertext = aead.encrypt(plaintext, ASSOCIATED_DATA)
        val encoded = Base64.encodeToString(ciphertext, Base64.NO_WRAP)
        context.tokenDataStore.edit { it[ciphertextKey] = encoded }
    }

    suspend fun clear() {
        cached = null
        loaded.set(true)
        context.tokenDataStore.edit { it.remove(ciphertextKey) }
    }

    /** Eagerly load the persisted bundle into the cache (call on app start). */
    suspend fun load(): TokenBundle? {
        cached = context.tokenDataStore.data.first()[ciphertextKey]?.let(::decode)
        loaded.set(true)
        return cached
    }

    private fun read(): TokenBundle? {
        if (loaded.compareAndSet(false, true)) {
            cached = runBlocking {
                runCatching { context.tokenDataStore.data.first()[ciphertextKey]?.let(::decode) }.getOrNull()
            }
        }
        return cached
    }

    private fun decode(encoded: String): TokenBundle? = runCatching {
        val ciphertext = Base64.decode(encoded, Base64.NO_WRAP)
        val plaintext = aead.decrypt(ciphertext, ASSOCIATED_DATA)
        json.decodeFromString(TokenBundle.serializer(), String(plaintext))
    }.getOrNull()

    private companion object {
        val ASSOCIATED_DATA = "mycloud_tokens".toByteArray()

        fun buildAead(context: Context): Aead {
            AeadConfig.register()
            val handle = AndroidKeysetManager.Builder()
                .withSharedPref(context, "mycloud_token_keyset", "mycloud_keyset_prefs")
                .withKeyTemplate(KeyTemplates.get("AES256_GCM"))
                .withMasterKeyUri("android-keystore://mycloud_token_master_key")
                .build()
                .keysetHandle
            return handle.getPrimitive(Aead::class.java)
        }
    }
}
