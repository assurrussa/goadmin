<template>
  <h1 class="text-2xl font-semibold text-text-primary">Настройки пользователя</h1>
  <Dialog>
    <DialogTrigger as-child><AppButton>Открыть диалог</AppButton></DialogTrigger>
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Настройки пользователя</DialogTitle>
        <DialogDescription>Проверка формы и фокуса во всплывающем окне.</DialogDescription>
      </DialogHeader>
      <FormInput id="comparison-dialog-name" v-model="name" label="Имя в диалоге" />
      <DialogFooter>
        <DialogClose as-child><AppButton>Готово</AppButton></DialogClose>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import AppButton from '@/components/ui/AppButton.vue'
import FormInput from '@/components/form/FormInput.vue'
import {
  Dialog,
  DialogTrigger,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogClose,
} from '@/components/ui/dialog'

const name = ref('Анна Волкова')
</script>
