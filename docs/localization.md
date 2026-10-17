# Admin interface languages

The built-in UI supports Russian and English. The selector is available in the
admin header and on sign-in screens. The choice is stored in `goadmin.locale` in
browser local storage; blocked storage keeps a session-only choice. Unsupported
or corrupt values fall back to Russian. Locale affects presentation only:
server validation, CSRF, canonical identities, permissions and commands use the
same contracts in both languages. User names, email addresses, field values and
table cell content are not translated.

Consumer extensions can register explicit UI messages without replacing the
core dictionary:

```ts
import type { AdminExtension } from '@admin-core/extensions'
import { useI18n } from '@admin-core/i18n'

export default {
  translations: {
    ru: { 'orders.title': 'Заказы' },
    en: { 'orders.title': 'Orders' },
  },
} satisfies AdminExtension

// In an extension component:
const { t } = useI18n()
// {{ t('orders.title', 'Заказы') }}
```

`registerTranslations` is also available for explicit registration from an
extension install hook. Messages are plain text, rendered with normal Vue
escaping. Optional `{name}` interpolation is plain text, never HTML. Missing
messages fall back to the Russian entry, an explicit fallback, then the key.
Consumer menu labels use explicit dictionary entries; register custom labels in
both languages. Arbitrary server messages remain visible if no dictionary entry
exists; do not replace an unknown security outcome with a success or disabled
state.
