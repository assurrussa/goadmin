<script lang="ts">
import { Primitive, type PrimitiveProps } from 'reka-ui'
import {
  cloneVNode,
  defineComponent,
  Fragment,
  h,
  isVNode,
  type VNode,
  type HTMLAttributes,
  type PropType,
} from 'vue'
import { cn } from '@/lib/utils'
import { buttonVariants, type ButtonVariants } from '.'

export default defineComponent({
  inheritAttrs: false,
  props: {
    as: { type: [String, Object] as PropType<PrimitiveProps['as']>, default: 'button' },
    asChild: Boolean,
    disabled: Boolean,
    variant: String as PropType<ButtonVariants['variant']>,
    size: String as PropType<ButtonVariants['size']>,
    class: [String, Array, Object] as PropType<HTMLAttributes['class']>,
  },
  setup(props, { attrs, slots }) {
    function suppressActivation(event: Event) {
      if (!props.disabled) return
      if (event instanceof KeyboardEvent && !['Enter', ' '].includes(event.key)) return
      event.preventDefault()
      event.stopImmediatePropagation()
    }

    function flatten(children: VNode[] = []): VNode[] {
      return children.flatMap((child) =>
        child.type === Fragment && Array.isArray(child.children)
          ? flatten(child.children.filter(isVNode))
          : [child],
      )
    }

    return () => {
      const disabledAttrs = props.disabled
        ? { disabled: true, 'aria-disabled': 'true', 'data-disabled': '', tabindex: -1 }
        : {}
      return h(
        Primitive,
        {
          ...attrs,
          ...disabledAttrs,
          as: props.as,
          asChild: props.asChild,
          'data-ui': 'button',
          'data-part': 'root',
          class: cn(buttonVariants({ variant: props.variant, size: props.size }), props.class),
          ...(props.disabled
            ? {
                onClickCapture: suppressActivation,
                onKeydownCapture: suppressActivation,
                onKeyupCapture: suppressActivation,
              }
            : {}),
        },
        {
          default: () => {
            const children = props.asChild ? flatten(slots.default?.()) : slots.default?.()
            // Reka Slot lets child attributes win. Fence a disabled slotted link
            // as well, including an explicit child tabindex or disabled=false.
            return props.asChild && props.disabled
              ? children?.map((child) => cloneVNode(child, disabledAttrs))
              : children
          },
        },
      )
    }
  },
})
</script>
