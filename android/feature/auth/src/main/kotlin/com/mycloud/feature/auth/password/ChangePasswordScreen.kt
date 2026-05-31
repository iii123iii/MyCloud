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
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle

@Composable
fun ChangePasswordScreen(
    modifier: Modifier = Modifier,
    viewModel: ChangePasswordViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()

    Surface(modifier = modifier.fillMaxSize()) {
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
            val pwKeyboard = KeyboardOptions(keyboardType = KeyboardType.Password)

            OutlinedTextField(
                value = state.oldPassword,
                onValueChange = viewModel::onOldChange,
                label = { Text("Current password") },
                singleLine = true,
                enabled = !state.isSubmitting,
                visualTransformation = PasswordVisualTransformation(),
                keyboardOptions = pwKeyboard,
                modifier = fieldModifier,
            )
            Spacer(Modifier.height(12.dp))
            OutlinedTextField(
                value = state.newPassword,
                onValueChange = viewModel::onNewChange,
                label = { Text("New password (min 8 chars)") },
                singleLine = true,
                enabled = !state.isSubmitting,
                visualTransformation = PasswordVisualTransformation(),
                keyboardOptions = pwKeyboard,
                modifier = fieldModifier,
            )
            Spacer(Modifier.height(12.dp))
            OutlinedTextField(
                value = state.confirmPassword,
                onValueChange = viewModel::onConfirmChange,
                label = { Text("Confirm new password") },
                singleLine = true,
                enabled = !state.isSubmitting,
                visualTransformation = PasswordVisualTransformation(),
                keyboardOptions = pwKeyboard,
                isError = state.errorMessage != null,
                modifier = fieldModifier,
            )

            if (state.errorMessage != null) {
                Spacer(Modifier.height(8.dp))
                Text(
                    text = state.errorMessage!!,
                    color = MaterialTheme.colorScheme.error,
                    style = MaterialTheme.typography.bodySmall,
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
                    CircularProgressIndicator(
                        modifier = Modifier.size(18.dp),
                        strokeWidth = 2.dp,
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
