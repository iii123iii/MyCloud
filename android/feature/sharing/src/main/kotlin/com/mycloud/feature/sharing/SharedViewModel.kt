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
    val isLoading: Boolean = false,
    val error: String? = null,
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
        _state.update { it.copy(isLoading = true, error = null) }
        viewModelScope.launch {
            val sharesResult = shareRepository.listShares()
            val grantsResult = shareRepository.listGrants()
            val shares = (sharesResult as? NetworkResult.Success)?.data.orEmpty()
            val grants = (grantsResult as? NetworkResult.Success)?.data.orEmpty()
            // Surface a real failure instead of showing it as an empty list.
            val error = sharesResult.userMessageOrNull() ?: grantsResult.userMessageOrNull()
            _state.update {
                it.copy(shares = shares, grants = grants, isLoading = false, error = error)
            }
        }
    }

    fun revokeShare(id: String) {
        viewModelScope.launch {
            shareRepository.deleteShare(id)
            refresh()
        }
    }

    fun revokeGrant(id: String) {
        viewModelScope.launch {
            shareRepository.deleteGrant(id)
            refresh()
        }
    }

    fun publicLink(token: String): String = shareRepository.publicLink(token)
}
