package com.mycloud.core.model

/** Top-level authentication state that drives the root navigation gate. */
enum class AuthState {
    /** Session restore in progress (splash is held). */
    LOADING,

    /** No valid session — show Login. */
    LOGGED_OUT,

    /** Authenticated but the server requires a password change first. */
    NEEDS_PASSWORD_CHANGE,

    /** App-lock engaged — require biometric/credential unlock. */
    LOCKED,

    /** Fully authenticated and unlocked — show the app. */
    AUTHENTICATED,
}
