/** Display name for a site role; the built-in roles are stored lowercase. */
export function siteRoleLabel(name: string): string {
  if (name === 'admin') return 'Admin'
  if (name === 'user') return 'User'
  return name
}
