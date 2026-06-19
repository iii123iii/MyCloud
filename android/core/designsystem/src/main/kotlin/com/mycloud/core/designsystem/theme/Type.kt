package com.mycloud.core.designsystem.theme

import androidx.compose.material3.Typography
import androidx.compose.ui.text.style.LineHeightStyle
import androidx.compose.ui.text.style.LineHeightStyle.Alignment
import androidx.compose.ui.text.style.LineHeightStyle.Trim

/**
 * Material default type scale (Roboto / system), per the chosen design
 * direction. Kept as a named value so a future tweak (e.g. applying the
 * Expressive "emphasized" roles to titles) lives in one place.
 *
 * Body and label roles get a centered [LineHeightStyle] with no first/last-line
 * trim so multi-line text stays evenly spaced and legible at large font scales
 * (accessibility), instead of cramming lines together.
 */
private val readableLineHeight = LineHeightStyle(
    alignment = Alignment.Center,
    trim = Trim.None,
)

private val base = Typography()

val MyCloudTypography = base.copy(
    bodyLarge = base.bodyLarge.copy(lineHeightStyle = readableLineHeight),
    bodyMedium = base.bodyMedium.copy(lineHeightStyle = readableLineHeight),
    bodySmall = base.bodySmall.copy(lineHeightStyle = readableLineHeight),
    labelLarge = base.labelLarge.copy(lineHeightStyle = readableLineHeight),
    labelMedium = base.labelMedium.copy(lineHeightStyle = readableLineHeight),
    labelSmall = base.labelSmall.copy(lineHeightStyle = readableLineHeight),
)
