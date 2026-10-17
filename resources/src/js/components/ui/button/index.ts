import type { VariantProps } from 'class-variance-authority'
import { cva } from 'class-variance-authority'

export { default as Button } from './Button.vue'

export const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-[var(--goadmin-control-radius,var(--radius-md,0.375rem))] text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50 cursor-pointer [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0',
  {
    variants: {
      variant: {
        default: 'bg-primary text-primary-foreground shadow hover:bg-primary/90',
        destructive: 'bg-destructive text-destructive-foreground shadow-sm hover:bg-destructive/90',
        outline:
          'border-2 border-input bg-transparent text-foreground shadow-sm hover:bg-accent hover:text-accent-foreground',
        secondary: 'bg-secondary text-secondary-foreground shadow-sm hover:bg-secondary/80',
        ghost: 'hover:bg-accent hover:text-accent-foreground',
        link: 'text-primary underline-offset-4 hover:underline',
      },
      size: {
        default: 'h-[var(--goadmin-control-height,2.25rem)] px-4 py-2',
        xs: 'h-[var(--goadmin-control-height-xs,1.75rem)] rounded-[var(--goadmin-control-radius-xs,0.25rem)] px-2',
        sm: 'h-[var(--goadmin-control-height-sm,2rem)] rounded-[var(--goadmin-control-radius,var(--radius-md,0.375rem))] px-3 text-xs',
        lg: 'h-[var(--goadmin-control-height-lg,2.5rem)] rounded-[var(--goadmin-control-radius,var(--radius-md,0.375rem))] px-8',
        icon: 'h-[var(--goadmin-control-height,2.25rem)] w-[var(--goadmin-control-height,2.25rem)]',
        'icon-sm': 'size-[var(--goadmin-control-height-sm,2rem)]',
        'icon-lg': 'size-[var(--goadmin-control-height-lg,2.5rem)]',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
)

export type ButtonVariants = VariantProps<typeof buttonVariants>
