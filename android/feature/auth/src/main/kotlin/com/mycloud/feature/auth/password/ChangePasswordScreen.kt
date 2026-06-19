@file:OptIn(androidx.compose.material3.ExperimentalMaterial3ExpressiveApi::class)

package com.mycloud.feature.auth.password

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Visibility
import androidx.compose.material.icons.filled.VisibilityOff
import androidx.compose.material3.Button
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle

@Composable
fun ChangePasswordScreen(
    modifier: Modifier = Modifier,
    viewModel: ChangePasswordViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }
    var oldVisible by remember { mutableStateOf(false) }
    var newVisible by remember { mutableStateOf(false) }
    var confirmVisible by remember { mutableStateOf(false) }

    // Confirm the change before the gate navigates away on success.
    LaunchedEffect(state.success) {
        if (state.success) snackbarHostState.showSnackbar("Password updated")
    }

    Scaffold(
        modifier = modifier.fillMaxSize(),
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Surface(modifier = Modifier.fillMaxSize().padding(padding)) {
            Column(
                modifier = Modifier
                    .fillMaxSize()
                    .statusBarsPadding()
                    .verticalScroll(rememberScrollState())
                    .imePadding()
                    .padding(24.dp),
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.Center,
            ) {
                Text("Change your password", style = MaterialTheme.typography.headlineSmall)
                Text(
                    "You must set a new password before continuing.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Spacer(Modifier.height(20.dp))

                val fieldModifier = Modifier
                    .fillMaxWidth()
                    .widthIn(max = 420.dp)

                OutlinedTextField(
                    value = state.oldPassword,
                    onValueChange = viewModel::onOldChange,
                    label = { Text("Current password") },
                    singleLine = true,
                    enabled = !state.isSubmitting,
                    visualTransformation = if (oldVisible) VisualTransformation.None else PasswordVisualTransformation(),
                    trailingIcon = { PasswordToggle(oldVisible) { oldVisible = !oldVisible } },
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Next),
                    modifier = fieldModifier,
                )
                Spacer(Modifier.height(12.dp))
                OutlinedTextField(
                    value = state.newPassword,
                    onValueChange = viewModel::onNewChange,
                    label = { Text("New password (min 8 chars)") },
                    singleLine = true,
                    enabled = !state.isSubmitting,
                    visualTransformation = if (newVisible) VisualTransformation.None else PasswordVisualTransformation(),
                    trailingIcon = { PasswordToggle(newVisible) { newVisible = !newVisible } },
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Next),
                    modifier = fieldModifier,
                )
                Spacer(Modifier.height(12.dp))
                OutlinedTextField(
                    value = state.confirmPassword,
                    onValueChange = viewModel::onConfirmChange,
                    label = { Text("Confirm new password") },
                    singleLine = true,
                    enabled = !state.isSubmitting,
                    visualTransformation = if (confirmVisible) VisualTransformation.None else PasswordVisualTransformation(),
                    trailingIcon = { PasswordToggle(confirmVisible) { confirmVisible = !confirmVisible } },
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Done),
                    keyboardActions = KeyboardActions(onDone = { viewModel.submit() }),
                    isError = state.passwordsMismatch || state.errorMessage != null,
                    supportingText = if (state.passwordsMismatch) {
                        { Text("New passwords don't match.") }
                    } else null,
                    modifier = fieldModifier,
                )

                if (state.errorMessage != null) {
                    Spacer(Modifier.height(8.dp))
                    Text(
                        text = state.errorMessage!!,
                        color = MaterialTheme.colorScheme.error,
                        style = MaterialTheme.typography.bodySmall,
                        modifier = fieldModifier.semantics { liveRegion = LiveRegionMode.Polite },
                    )
                }

                Spacer(Modifier.height(20.dp))
                Button(
                    onClick = viewModel::submit,
                    enabled = state.canSubmit,
                    modifier = Modifier
                        .fillMaxWidth()
                        .widthIn(max = 420.dp),
                ) {
                    if (state.isSubmitting) {
                        LoadingIndicator(
                            modifier = Modifier.size(18.dp),
                            color = MaterialTheme.colorScheme.onPrimary,
                        )
                    } else {
                        Text("Update password")
                    }
                }
                Spacer(Modifier.height(8.dp))
                TextButton(onClick = viewModel::logout, enabled = !state.isSubmitting) {
                    Text("Sign out")
                }
            }
        }
    }
}

@Composable
private fun PasswordToggle(visible: Boolean, onToggle: () -> Unit) {
    IconButton(onClick = onToggle) {
        Icon(
            imageVector = if (visible) Icons.Filled.VisibilityOff else Icons.Filled.Visibility,
            contentDescription = if (visible) "Hide password" else "Show password",
        )
    }
}
