package com.mycloud.feature.auth.lock

import android.content.Context
import android.content.ContextWrapper
import android.os.Build
import androidx.biometric.BiometricManager
import androidx.biometric.BiometricManager.Authenticators.BIOMETRIC_STRONG
import androidx.biometric.BiometricManager.Authenticators.DEVICE_CREDENTIAL
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
 * The allowed authenticators for the unlock prompt. Strong biometrics OR the device
 * credential (PIN/pattern/password). Combining the two via setNegativeButtonText is
 * only supported below API 30; from API 30 the device-credential button is built in.
 */
private val allowedAuthenticators = BIOMETRIC_STRONG or DEVICE_CREDENTIAL

/**
 * Shows a strong-biometric (or device-credential) unlock prompt.
 *
 * Unlike a permissive gate, when the device can authenticate by NEITHER biometric NOR
 * device credential we do NOT silently unlock — that would let any unsecured device walk
 * past the lock. Instead [onUnavailable] is invoked so the caller can surface the problem
 * (e.g. prompt the user to set a screen lock, or route to logout).
 */
fun promptBiometricUnlock(
    activity: FragmentActivity,
    onSuccess: () -> Unit,
    onError: (String) -> Unit,
    onUnavailable: (String) -> Unit,
) {
    val manager = BiometricManager.from(activity)
    when (manager.canAuthenticate(allowedAuthenticators)) {
        BiometricManager.BIOMETRIC_SUCCESS -> Unit // proceed to prompt below
        BiometricManager.BIOMETRIC_ERROR_NONE_ENROLLED ->
            return onUnavailable("No screen lock set up. Add a fingerprint, face, or PIN in system settings to unlock MyCloud.")
        BiometricManager.BIOMETRIC_ERROR_NO_HARDWARE,
        BiometricManager.BIOMETRIC_ERROR_HW_UNAVAILABLE,
        BiometricManager.BIOMETRIC_ERROR_UNSUPPORTED ->
            return onUnavailable("This device can't verify it's you. Sign out and back in to continue.")
        else ->
            return onUnavailable("Unlock isn't available right now. Try again, or sign out.")
    }
    val prompt = BiometricPrompt(
        activity,
        ContextCompat.getMainExecutor(activity),
        object : BiometricPrompt.AuthenticationCallback() {
            override fun onAuthenticationSucceeded(result: BiometricPrompt.AuthenticationResult) {
                onSuccess()
            }

            override fun onAuthenticationError(errorCode: Int, errString: CharSequence) {
                if (errorCode == BiometricPrompt.ERROR_LOCKOUT_PERMANENT) {
                    onError("Biometrics are locked. Use your device PIN/password, or sign out.")
                } else {
                    onError(errString.toString())
                }
            }
        },
    )
    val builder = BiometricPrompt.PromptInfo.Builder()
        .setTitle("Unlock MyCloud")
        .setSubtitle("Confirm it's you to continue")
    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
        builder.setAllowedAuthenticators(allowedAuthenticators)
    } else {
        // Below API 30 a DEVICE_CREDENTIAL prompt can't also show a negative button,
        // so restrict to strong biometrics and provide an explicit cancel.
        builder.setAllowedAuthenticators(BIOMETRIC_STRONG)
        builder.setNegativeButtonText("Cancel")
    }
    prompt.authenticate(builder.build())
}
