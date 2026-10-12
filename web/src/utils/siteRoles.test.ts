import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { siteRoleLabel } from './siteRoles.ts'

describe('siteRoleLabel', () => {
  it('capitalizes the built-in roles', () => {
    assert.equal(siteRoleLabel('admin'), 'Admin')
    assert.equal(siteRoleLabel('user'), 'User')
  })
  it('leaves custom role names alone', () => {
    assert.equal(siteRoleLabel('Project Lead'), 'Project Lead')
    assert.equal(siteRoleLabel('qa'), 'qa')
  })
})
