package com.mycloud.feature.sharing

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.common.result.userMessageOrNull
import com.mycloud.core.model.Grant
import com.mycloud.core.model.Share
import com.mycloud.data.repository.ShareRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class SharedUiState(
    val shares: List<Share> = emptyList(),
    val grants: List<Grant> = emptyList(),
    // Grants shared WITH the current user (read-only; the recipient can't revoke them).
    val incomingGrants: List<Grant> = emptyList(),
    val isLoading: Boolean = false,
    // Per-list errors so a failure in one tab doesn't bleed into the others.
    val sharesError: String? = null,
    val grantsError: String? = null,
    val incomingGrantsError: String? = null,
)

@HiltViewModel
class SharedViewModel @Inject constructor(
    private val shareRepository: ShareRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(SharedUiState())
    val state: StateFlow<SharedUiState> = _state.asStateFlow()

    init {
        refresh()
    }

    fun refresh() {
        _state.update {
            it.copy(
                isLoading = true,
                sharesError = null,
                grantsError = null,
                incomingGrantsError = null,
            )
        }
        viewModelScope.launch {
            val sharesResult = shareRepository.listShares()
            val grantsResult = shareRepository.listGrants(direction = "outgoing")
            val incomingResult = shareRepository.listGrants(direction = "incoming")
            // Surface a real failure instead of showing it as an empty list, scoped
            // to the list that actually failed.
            _state.update { previous ->
                previous.copy(
                    shares = (sharesResult as? NetworkResult.Success)?.data ?: previous.shares,
                    grants = (grantsResult as? NetworkResult.Success)?.data ?: previous.grants,
                    incomingGrants = (incomingResult as? NetworkResult.Success)?.data ?: previous.incomingGrants,
                    isLoading = false,
                    sharesError = sharesResult.userMessageOrNull(),
                    grantsError = grantsResult.userMessageOrNull(),
                    incomingGrantsError = incomingResult.userMessageOrNull(),
                )
            }
        }
    }

    fun revokeShare(id: String) {
        viewModelScope.launch {
            when (val result = shareRepository.deleteShare(id)) {
                is NetworkResult.Success -> refresh()
                else -> _state.update { it.copy(sharesError = result.userMessageOrNull()) }
            }
        }
    }

    fun revokeGrant(id: String) {
        viewModelScope.launch {
            when (val result = shareRepository.deleteGrant(id)) {
                is NetworkResult.Success -> refresh()
                else -> _state.update { it.copy(grantsError = result.userMessageOrNull()) }
            }
        }
    }

    fun publicLink(token: String): String = shareRepository.publicLink(token)
}
