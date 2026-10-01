const path = require('path')

const projectRoot = path.resolve(__dirname, '..')
const defaultAdminSiteRoot = path.resolve(projectRoot, '..')
const defaultAdminExtRoot = path.join(defaultAdminSiteRoot, '_host')

const resolveConfiguredPath = (value, fallbackAbsolute) => {
  if (!value) {
    return fallbackAbsolute
  }
  if (path.isAbsolute(value)) {
    return value
  }
  return path.resolve(process.cwd(), value)
}

const adminSiteRoot = resolveConfiguredPath(process.env.VITE_ADMIN_SITE_ROOT, defaultAdminSiteRoot)
const adminExtRoot = resolveConfiguredPath(process.env.VITE_ADMIN_EXT_ROOT, defaultAdminExtRoot)
const adminExtRootConfigured = Boolean(process.env.VITE_ADMIN_EXT_ROOT)
const adminExtensionManifestPaths = (process.env.VITE_ADMIN_EXTENSION_MANIFESTS || '')
  .split(path.delimiter)
  .map((value) => value.trim())
  .filter(Boolean)
  .map((value) => resolveConfiguredPath(value, value))
const adminGeneratedRoot = path.join(projectRoot, '.admin-extensions')
const adminPublicRoot = resolveConfiguredPath(process.env.VITE_ADMIN_PUBLIC_ROOT, path.join(adminSiteRoot, 'public'))
const localAdminTsconfigPath = path.join(projectRoot, 'tsconfig.admin.json')
const legacyAdminTsconfigPath = resolveConfiguredPath(
  process.env.VITE_ADMIN_TSCONFIG_PATH,
  localAdminTsconfigPath
)

module.exports = {
  adminExtRoot,
  adminExtRootConfigured,
  adminExtensionManifestPaths,
  adminGeneratedRoot,
  adminPublicRoot,
  adminSiteRoot,
  defaultAdminExtRoot,
  defaultAdminSiteRoot,
  legacyAdminTsconfigPath,
  localAdminTsconfigPath,
  projectRoot,
  resolveConfiguredPath,
}
