package com.mycloud.feature.auth.lock

import android.content.Context
import android.content.ContextWrapper
import androidx.biometric.BiometricManager
import androidx.biometric.BiometricManager.Authenticators.BIOMETRIC_STRONG
import androidx.biometric.BiometricPrompt
import androidx.core.content.ContextCompat
import androidx.fragment.app.FragmentActivity

/** Walks the context chain to the hosting [FragmentActivity] (required by BiometricPrompt). */
fun Context.findFragmentActivity(): FragmentActivity? {
    var ctx: Context? = this
    while (ctx is ContextWrapper) {
        if (ctx is FragmentActivity) return ctx
        ctx = ctx.baseContext
    }
    return null
}

/**
 * Shows a strong-biometric unlock prompt. If no biometric is enrolled/available
 * the gate opens rather than locking the user out (Phase 1 behavior; a device-
 * credential fallback can be added once MainActivity hosts it on all API levels).
 */
fun promptBiometricUnlock(
    activity: FragmentActivity,
    onSuccess: () -> Unit,
    onError: (String) -> Unit,
) {
    val manager = BiometricManager.from(activity)
    if (manager.canAuthenticate(BIOMETRIC_STRONG) != BiometricManager.BIOMETRIC_SUCCESS) {
        onSuccess()
        return
    }
    val prompt = BiometricPrompt(
        activity,
        ContextCompat.getMainExecutor(activity),
        object : BiometricPrompt.AuthenticationCallback() {
            override fun onAuthenticationSucceeded(result: BiometricPrompt.AuthenticationResult) {
                onSuccess()
            }

            override fun onAuthenticationError(errorCode: Int, errString: CharSequence) {
                onError(errString.toString())
            }
        },
    )
    val info = BiometricPrompt.PromptInfo.Builder()
        .setTitle("Unlock MyCloud")
        .setSubtitle("Confirm it's you to continue")
        .setNegativeButtonText("Cancel")
        .setAllowedAuthenticators(BIOMETRIC_STRONG)
        .build()
    prompt.authenticate(info)
}
