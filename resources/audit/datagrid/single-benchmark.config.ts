import config from './benchmark.config'
export default {
  ...config,
  test: { ...config.test, include: ['audit/datagrid/render.single.test.ts'] },
}
