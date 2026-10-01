const fs = require('fs')
const path = require('path')
const {
  adminExtRoot,
  adminExtRootConfigured,
  adminExtensionManifestPaths,
  adminGeneratedRoot,
  defaultAdminExtRoot,
  legacyAdminTsconfigPath,
  localAdminTsconfigPath,
  projectRoot,
} = require('./admin-paths.cjs')
const { materializeAdminExtensions } = require('./materialize-admin-extensions.cjs')

const materialized = materializeAdminExtensions({
  generatedRoot: adminGeneratedRoot,
  manifestPaths: adminExtensionManifestPaths,
  legacyRoot: adminExtRoot,
  includeLegacy: adminExtRootConfigured,
})

const toPosix = (value) => value.split(path.sep).join('/')
const toRelativeSpecifier = (value) => {
  const posixValue = toPosix(value)
  if (posixValue.startsWith('.') || posixValue.startsWith('/')) {
    return posixValue
  }
  return `./${posixValue}`
}
const fallbackExtSrc = path.resolve(__dirname, '..', 'src/js/empty-ext')
const resolveExtSrc = (root) => {
  const candidate = path.join(root, 'src/js')
  if (fs.existsSync(candidate)) {
    return candidate
  }
  return fallbackExtSrc
}

const buildConfig = (targetPath, extSrc) => {
  const tsconfigDir = path.dirname(targetPath)
  const baseConfigPath = path.join(projectRoot, 'tsconfig.paths.json')
  const baseConfigRel = toRelativeSpecifier(path.relative(tsconfigDir, baseConfigPath))
  const extSrcRel = toPosix(path.relative(tsconfigDir, extSrc))
  const coreEnvRel = toPosix(path.relative(tsconfigDir, path.join(projectRoot, 'env.d.ts')))
  const coreSrcRel = toPosix(path.relative(tsconfigDir, path.join(projectRoot, 'src')))
  const coreJsRel = toPosix(path.relative(tsconfigDir, path.join(projectRoot, 'src/js')))
  const generatedRootRel = toPosix(path.relative(tsconfigDir, materialized.generatedRoot))
  const generatedModuleRel = toPosix(path.relative(tsconfigDir, materialized.generatedModulePath))

  return {
    extends: baseConfigRel,
    include: [
      coreEnvRel,
      `${coreSrcRel}/**/*`,
      `${coreSrcRel}/**/*.vue`,
      `${extSrcRel}/**/*`,
      `${generatedRootRel}/**/*`,
    ],
    exclude: [`${coreSrcRel}/**/__tests__/*`],
    compilerOptions: {
      baseUrl: '.',
      paths: {
        '@/*': [`${coreJsRel}/*`],
        '~/*': [`${coreJsRel}/*`],
        '@admin-core/*': [`${coreJsRel}/*`],
        '@admin-ext/*': [`${extSrcRel}/*`],
        '@admin-extensions-generated': [generatedModuleRel],
      },
    },
  }
}

const writeConfig = (targetPath) => {
  const config = buildConfig(targetPath, resolveExtSrc(defaultAdminExtRoot))
  fs.mkdirSync(path.dirname(targetPath), { recursive: true })
  fs.writeFileSync(targetPath, JSON.stringify(config, null, 2) + '\n')
}

const writeCompatibilityConfig = (targetPath) => {
  const config = buildConfig(targetPath, resolveExtSrc(adminExtRoot))
  fs.mkdirSync(path.dirname(targetPath), { recursive: true })
  fs.writeFileSync(targetPath, JSON.stringify(config, null, 2) + '\n')
}

writeConfig(localAdminTsconfigPath)
if (path.resolve(localAdminTsconfigPath) !== path.resolve(legacyAdminTsconfigPath)) {
  writeCompatibilityConfig(legacyAdminTsconfigPath)
}

console.log(`admin tsconfig paths updated: ${localAdminTsconfigPath}`)
if (path.resolve(localAdminTsconfigPath) !== path.resolve(legacyAdminTsconfigPath)) {
  console.log(`admin tsconfig compatibility copy updated: ${legacyAdminTsconfigPath}`)
}
