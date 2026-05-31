package com.mycloud.core.designsystem.theme

import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.ui.graphics.Color

/**
 * MyCloud brand color.
 *
 * Seeded from the web app's one chromatic accent — `oklch(0.488 0.243 264.376)`
 * in `frontend/app/globals.css` ≈ `#5B47D6` (indigo/violet). The role colors
 * below are a tonal palette derived from that seed (Material Theme Builder
 * style); the neutral roles are left to Material's baseline. Regenerate with the
 * Material Theme Builder if you re-tune the seed, and keep this the single
 * source of brand identity (dynamic color is intentionally NOT the default).
 */
val BrandSeed = Color(0xFF5B47D6)

// ---- Light ----
private val md_primary_light = Color(0xFF4B3FBE)
private val md_onPrimary_light = Color(0xFFFFFFFF)
private val md_primaryContainer_light = Color(0xFFE3DFFF)
private val md_onPrimaryContainer_light = Color(0xFF150068)
private val md_secondary_light = Color(0xFF5D5C72)
private val md_onSecondary_light = Color(0xFFFFFFFF)
private val md_secondaryContainer_light = Color(0xFFE3E0F9)
private val md_onSecondaryContainer_light = Color(0xFF1A1A2C)
private val md_tertiary_light = Color(0xFF79536A)
private val md_onTertiary_light = Color(0xFFFFFFFF)
private val md_tertiaryContainer_light = Color(0xFFFFD8EB)
private val md_onTertiaryContainer_light = Color(0xFF2D1124)
private val md_error_light = Color(0xFFBA1A1A)
private val md_onError_light = Color(0xFFFFFFFF)
private val md_errorContainer_light = Color(0xFFFFDAD6)
private val md_onErrorContainer_light = Color(0xFF410002)

// ---- Dark ----
private val md_primary_dark = Color(0xFFC6BFFF)
private val md_onPrimary_dark = Color(0xFF1E0C8E)
private val md_primaryContainer_dark = Color(0xFF3A2FA6)
private val md_onPrimaryContainer_dark = Color(0xFFE3DFFF)
private val md_secondary_dark = Color(0xFFC7C4DD)
private val md_onSecondary_dark = Color(0xFF2F2F42)
private val md_secondaryContainer_dark = Color(0xFF454559)
private val md_onSecondaryContainer_dark = Color(0xFFE3E0F9)
private val md_tertiary_dark = Color(0xFFE9B9D2)
private val md_onTertiary_dark = Color(0xFF452639)
private val md_tertiaryContainer_dark = Color(0xFF5F3C52)
private val md_onTertiaryContainer_dark = Color(0xFFFFD8EB)
private val md_error_dark = Color(0xFFFFB4AB)
private val md_onError_dark = Color(0xFF690005)
private val md_errorContainer_dark = Color(0xFF93000A)
private val md_onErrorContainer_dark = Color(0xFFFFDAD6)

val LightColors = lightColorScheme(
    primary = md_primary_light,
    onPrimary = md_onPrimary_light,
    primaryContainer = md_primaryContainer_light,
    onPrimaryContainer = md_onPrimaryContainer_light,
    secondary = md_secondary_light,
    onSecondary = md_onSecondary_light,
    secondaryContainer = md_secondaryContainer_light,
    onSecondaryContainer = md_onSecondaryContainer_light,
    tertiary = md_tertiary_light,
    onTertiary = md_onTertiary_light,
    tertiaryContainer = md_tertiaryContainer_light,
    onTertiaryContainer = md_onTertiaryContainer_light,
    error = md_error_light,
    onError = md_onError_light,
    errorContainer = md_errorContainer_light,
    onErrorContainer = md_onErrorContainer_light,
)

val DarkColors = darkColorScheme(
    primary = md_primary_dark,
    onPrimary = md_onPrimary_dark,
    primaryContainer = md_primaryContainer_dark,
    onPrimaryContainer = md_onPrimaryContainer_dark,
    secondary = md_secondary_dark,
    onSecondary = md_onSecondary_dark,
    secondaryContainer = md_secondaryContainer_dark,
    onSecondaryContainer = md_onSecondaryContainer_dark,
    tertiary = md_tertiary_dark,
    onTertiary = md_onTertiary_dark,
    tertiaryContainer = md_tertiaryContainer_dark,
    onTertiaryContainer = md_onTertiaryContainer_dark,
    error = md_error_dark,
    onError = md_onError_dark,
    errorContainer = md_errorContainer_dark,
    onErrorContainer = md_onErrorContainer_dark,
)
