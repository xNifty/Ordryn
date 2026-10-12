<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { api } from '@/api/client'
import type { AdminUser, SiteRole } from '@/api/types'
import { APIError } from '@/api/types'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { useAuth } from '@/composables/useAuth'
import { siteRoleLabel } from '@/utils/siteRoles'
import AdminSubnav from '@/components/AdminSubnav.vue'

const toast = useToast()
const { askConfirm } = useConfirm()
const { user: me } = useAuth()
const users = ref<AdminUser[]>([])
const siteRoles = ref<SiteRole[]>([])
const search = ref('')
const editingUsernameId = ref<number | null>(null)
const editUsernameValue = ref('')

const filteredUsers = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return users.value
  return users.value.filter((u) => {
    const name = (u.user_name || '').toLowerCase()
    const email = (u.email || '').toLowerCase()
    return name.includes(q) || email.includes(q)
  })
})

async function load() {
  try {
    const [list, roles] = await Promise.all([api.listAdminUsers(), api.listSiteRoles()])
    users.value = list
    siteRoles.value = roles.roles || []
  } catch (err) {
    toast.push(err instanceof APIError ? err.message : 'Failed to load users', 'error')
  }
}

function grantsAdmin(roleId: number) {
  return !!siteRoles.value.find((r) => r.id === roleId)?.permissions.includes('admin')
}

async function changeRole(user: AdminUser, event: Event) {
  const select = event.target as HTMLSelectElement
  const roleId = Number(select.value)
  const next = siteRoles.value.find((r) => r.id === roleId)
  if (!next || roleId === user.role_id) return
  const who = user.user_name || user.email
  // Gaining or losing admin is the one change worth a second look.
  if (grantsAdmin(roleId) !== grantsAdmin(user.role_id)) {
    const ok = await askConfirm({
      title: grantsAdmin(roleId) ? 'Make admin?' : 'Remove admin?',
      message: grantsAdmin(roleId)
        ? `${who} will be able to change every site setting, role, and account.`
        : `${who} will lose access to the admin area.`,
      confirmLabel: grantsAdmin(roleId) ? 'Make admin' : 'Remove admin',
      danger: true,
    })
    if (!ok) {
      select.value = String(user.role_id)
      return
    }
  }
  try {
    await api.setAdminUserRole(user.id, roleId)
    toast.push(`${who} is now ${siteRoleLabel(next.name)}`, 'success')
    await load()
  } catch (err) {
    select.value = String(user.role_id)
    toast.push(err instanceof APIError ? err.message : 'Could not change role', 'error')
  }
}

async function toggleBan(user: AdminUser) {
  const ok = await askConfirm({
    title: user.is_banned ? 'Unban user?' : 'Ban user?',
    message: user.is_banned
      ? `Unban ${user.email}?`
      : `Ban ${user.email}? They will no longer be able to sign in.`,
    confirmLabel: user.is_banned ? 'Unban' : 'Ban',
    danger: !user.is_banned,
  })
  if (!ok) return
  try {
    if (user.is_banned) {
      await api.unbanUser(user.id)
      toast.push('User unbanned', 'success')
    } else {
      await api.banUser(user.id)
      toast.push('User banned', 'info')
    }
    await load()
  } catch (err) {
    toast.push(err instanceof APIError ? err.message : 'Update failed', 'error')
  }
}

function startEditUsername(user: AdminUser) {
  editingUsernameId.value = user.id
  editUsernameValue.value = user.user_name || ''
}

function cancelEditUsername() {
  editingUsernameId.value = null
  editUsernameValue.value = ''
}

async function saveUsername(user: AdminUser) {
  const next = editUsernameValue.value.trim()
  if (!next) {
    toast.push('Username is required', 'error')
    return
  }
  try {
    await api.setAdminUsername(user.id, next)
    toast.push('Username updated', 'success')
    cancelEditUsername()
    await load()
  } catch (err) {
    toast.push(err instanceof APIError ? err.message : 'Failed to update username', 'error')
  }
}

onMounted(load)
</script>

<template>
  <div class="container mt-3">
    <AdminSubnav />
    <h1>Users</h1>
    <p class="text-muted">
      Search, rename, ban, or change the <RouterLink to="/admin/site-roles">site role</RouterLink> of accounts on this site.
    </p>

    <div class="mb-3" style="max-width: 28rem">
      <label for="admin-user-search" class="form-label">Search</label>
      <div class="position-relative">
        <i class="bi bi-search position-absolute top-50 start-0 translate-middle-y ms-3 text-muted" />
        <input
          id="admin-user-search"
          v-model="search"
          type="search"
          class="form-control ps-5"
          placeholder="Username or email"
          autocomplete="off"
        />
      </div>
    </div>

    <div class="card">
      <ul class="list-group list-group-flush">
        <li v-for="user in filteredUsers" :key="user.id" class="list-group-item">
          <div class="d-flex justify-content-between align-items-start gap-3 flex-wrap">
            <div class="flex-grow-1">
              <template v-if="editingUsernameId === user.id">
                <div class="input-group input-group-sm" style="max-width: 280px">
                  <input
                    v-model="editUsernameValue"
                    type="text"
                    class="form-control"
                    minlength="3"
                    maxlength="32"
                    pattern="[A-Za-z0-9_]+"
                    @keyup.enter="saveUsername(user)"
                  />
                  <button type="button" class="btn btn-primary" @click="saveUsername(user)">Save</button>
                  <button type="button" class="btn btn-outline-secondary" @click="cancelEditUsername">Cancel</button>
                </div>
              </template>
              <template v-else>
                <strong>{{ user.user_name || user.email }}</strong>
                <button type="button" class="btn btn-link btn-sm py-0" @click="startEditUsername(user)">
                  Edit username
                </button>
              </template>
              <div class="text-muted small">
                {{ user.email }}
                <span v-if="user.is_banned">· banned</span>
              </div>
            </div>
            <div class="d-flex align-items-center gap-2">
              <label class="visually-hidden" :for="`admin-user-role-${user.id}`">Site role</label>
              <select
                :id="`admin-user-role-${user.id}`"
                class="form-select form-select-sm"
                style="width: auto"
                :value="user.role_id"
                :disabled="user.id === me?.id"
                :title="user.id === me?.id ? 'You can\'t change your own role' : 'Site role'"
                @change="changeRole(user, $event)"
              >
                <option v-for="role in siteRoles" :key="role.id" :value="role.id">{{ siteRoleLabel(role.name) }}</option>
              </select>
              <button
                type="button"
                class="btn btn-sm"
                :class="user.is_banned ? 'btn-outline-secondary' : 'btn-outline-danger'"
                @click="toggleBan(user)"
              >
                {{ user.is_banned ? 'Unban' : 'Ban' }}
              </button>
            </div>
          </div>
        </li>
        <li v-if="!filteredUsers.length" class="list-group-item text-muted">
          {{ users.length ? 'No users match that search.' : 'No users found.' }}
        </li>
      </ul>
    </div>
  </div>
</template>
