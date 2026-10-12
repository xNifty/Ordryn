<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { api } from '@/api/client'
import type { ProjectPermInfo, SiteRole } from '@/api/types'
import { APIError } from '@/api/types'
import AdminSubnav from '@/components/AdminSubnav.vue'
import RolePermissionFields from '@/components/RolePermissionFields.vue'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { siteRoleLabel } from '@/utils/siteRoles'

const toast = useToast()
const { askConfirm } = useConfirm()
const roles = ref<SiteRole[]>([])
const catalog = ref<ProjectPermInfo[]>([])
const loaded = ref(false)
const saving = ref(false)
const editingId = ref<number | null>(null)
const formName = ref('')
const formPerms = ref<string[]>([])

const editing = computed(() => roles.value.find((r) => r.id === editingId.value) || null)
const isUserRole = computed(() => editing.value?.name === 'user')

function permLabels(role: SiteRole) {
  if (role.locked) return 'All permissions'
  const labels = catalog.value.filter((p) => role.permissions.includes(p.id)).map((p) => p.label)
  return labels.length ? labels.join(', ') : 'No site permissions'
}

async function load() {
  try {
    const data = await api.listSiteRoles()
    roles.value = data.roles || []
    catalog.value = data.catalog || []
  } catch (err) {
    toast.push(err instanceof APIError ? err.message : 'Failed to load site roles', 'error')
  } finally {
    loaded.value = true
  }
}

function resetForm() {
  editingId.value = null
  formName.value = ''
  formPerms.value = []
}

function startEdit(role: SiteRole) {
  editingId.value = role.id
  formName.value = role.name
  formPerms.value = role.permissions.filter((p) => catalog.value.some((c) => c.id === p))
}

async function save() {
  const name = formName.value.trim()
  if (!name) return
  saving.value = true
  try {
    if (editing.value) {
      await api.updateSiteRole(editing.value.id, {
        ...(isUserRole.value ? {} : { name }),
        permissions: formPerms.value,
      })
      toast.push('Role updated', 'success')
    } else {
      await api.createSiteRole({ name, permissions: formPerms.value })
      toast.push('Role created', 'success')
    }
    resetForm()
    await load()
  } catch (err) {
    toast.push(err instanceof APIError ? err.message : 'Could not save role', 'error')
  } finally {
    saving.value = false
  }
}

async function remove(role: SiteRole) {
  const ok = await askConfirm({
    title: `Delete ${role.name}?`,
    message: role.user_count
      ? `${role.user_count} ${role.user_count === 1 ? 'account uses' : 'accounts use'} this role and will move to User.`
      : 'No accounts use this role.',
    confirmLabel: 'Delete role',
    danger: true,
  })
  if (!ok) return
  try {
    const res = await api.deleteSiteRole(role.id)
    toast.push(res.users_moved ? `Role deleted; ${res.users_moved} moved to User` : 'Role deleted', 'success')
    if (editingId.value === role.id) resetForm()
    await load()
  } catch (err) {
    toast.push(err instanceof APIError ? err.message : 'Could not delete role', 'error')
  }
}

onMounted(load)
</script>

<template>
  <div class="container mt-3">
    <AdminSubnav />
    <h1>Site roles</h1>
    <p class="text-muted">
      Site roles decide what an account can do across the whole site, like starting projects or
      organizations. They're separate from the roles people hold inside a project or organization.
      Assign them on the <RouterLink to="/admin/users">Users</RouterLink> page.
    </p>

    <div class="row g-4">
      <div class="col-lg-7">
        <div class="card">
          <ul class="list-group list-group-flush">
            <li v-for="role in roles" :key="role.id" class="list-group-item">
              <div class="d-flex flex-wrap justify-content-between align-items-start gap-2">
                <div>
                  <strong>{{ siteRoleLabel(role.name) }}</strong>
                  <span v-if="role.built_in" class="badge text-bg-info ms-1">built-in</span>
                  <div class="small text-muted">{{ permLabels(role) }}</div>
                  <div class="small text-muted">
                    {{ role.user_count }} {{ role.user_count === 1 ? 'account' : 'accounts' }}
                    <template v-if="role.name === 'user'"> · default for new accounts</template>
                  </div>
                </div>
                <div v-if="!role.locked" class="d-flex gap-2">
                  <button class="btn btn-sm btn-outline-secondary" type="button" @click="startEdit(role)">Edit</button>
                  <button
                    v-if="!role.built_in"
                    class="btn btn-sm btn-outline-danger"
                    type="button"
                    @click="remove(role)"
                  >
                    Delete
                  </button>
                </div>
              </div>
            </li>
            <li v-if="loaded && !roles.length" class="list-group-item text-muted">No site roles found.</li>
          </ul>
        </div>
      </div>

      <div class="col-lg-5">
        <form class="card card-body" @submit.prevent="save">
          <h2 class="h5">{{ editing ? `Edit ${siteRoleLabel(editing.name)}` : 'Create a site role' }}</h2>
          <div class="mb-3">
            <label class="form-label small mb-0" for="site-role-name">Name</label>
            <input
              id="site-role-name"
              v-model="formName"
              type="text"
              class="form-control"
              maxlength="50"
              required
              :disabled="isUserRole"
              placeholder="e.g. Project Lead"
            />
            <div v-if="isUserRole" class="form-text">The default role for new accounts can't be renamed.</div>
          </div>
          <RolePermissionFields v-model="formPerms" :catalog="catalog" />
          <div class="d-flex gap-2">
            <button class="btn btn-primary" type="submit" :disabled="saving || !formName.trim()">
              {{ editing ? 'Save role' : 'Create role' }}
            </button>
            <button v-if="editing" class="btn btn-outline-secondary" type="button" @click="resetForm">Cancel</button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>
