export interface AdminMenuItem {
  name: string
  href?: string
  children?: AdminMenuItem[]
}

export interface AdminMenu {
  sections?: { key: string; title?: string; items?: AdminMenuItem[] }[]
}

export function dashboardSections(menu?: AdminMenu) {
  return (menu?.sections ?? [])
    .map((section) => {
      const links: { name: string; href: string }[] = []
      const visit = (items: AdminMenuItem[]) => {
        for (const item of items) {
          if (
            item.href &&
            item.href !== '/' &&
            item.href.startsWith('/') &&
            !item.href.startsWith('//')
          ) {
            links.push({ name: item.name, href: item.href })
          }
          visit(item.children ?? [])
        }
      }
      visit(section.items ?? [])
      return { key: section.key, title: section.title, links }
    })
    .filter((section) => section.links.length > 0)
}
