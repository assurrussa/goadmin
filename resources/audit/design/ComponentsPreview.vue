<template>
  <div class="space-y-6">
    <h1 class="text-2xl font-semibold text-text-primary">Компоненты и состояния</h1>
    <Card class="p-6 space-y-4">
      <div class="flex flex-wrap gap-4">
        <AppStatus variant="success">Активен</AppStatus>
        <AppStatus variant="secondary">Неактивен</AppStatus>
        <AppStatus variant="warning">Ожидает</AppStatus>
        <AppStatus variant="danger">Ошибка</AppStatus>
        <AppStatus variant="info">Выполняется</AppStatus>
      </div>
      <div class="flex flex-wrap gap-2">
        <AppButton id="design-primary">Основное</AppButton>
        <AppButton id="design-success" variant="success">Успех</AppButton>
        <AppButton id="design-warning" variant="warning">Предупреждение</AppButton>
        <AppButton id="design-danger" variant="danger">Удаление</AppButton>
      </div>
      <FormInput id="design-name" v-model="name" label="Имя" />
      <FormInput id="design-error" label="Email" model-value="invalid" error="Проверьте адрес" />
      <FormInput
        id="design-readonly"
        label="Только чтение"
        model-value="Сохранённое значение"
        readonly
      />
      <FormInput
        id="design-disabled"
        label="Недоступно"
        model-value="Недоступное значение"
        disabled
      />
      <div style="--color-error: rgb(126, 34, 206)">
        <FormInput id="design-host-error" label="Цвет хоста" error="Ошибка с цветом хоста" />
      </div>
      <AppButton id="design-override" class="h-16 rounded-none">Переопределённый размер</AppButton>
      <Dialog>
        <DialogTrigger as-child><AppButton>Открыть диалог</AppButton></DialogTrigger>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Настройки пользователя</DialogTitle>
            <DialogDescription>Проверка формы и фокуса во всплывающем окне.</DialogDescription>
          </DialogHeader>
          <FormInput id="design-dialog-name" v-model="name" label="Имя в диалоге" />
          <DialogFooter>
            <DialogClose as-child><AppButton>Готово</AppButton></DialogClose>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
    <RichTextEditor v-model="document" :allow-file-upload="false" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppStatus from '@/components/ui/AppStatus.vue'
import FormInput from '@/components/form/FormInput.vue'
import RichTextEditor from '@/components/form/RichTextEditor.vue'
import { Card } from '@/components/ui/card'
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
const document = ref('<p>Описание пользователя и рабочие заметки.</p>')
</script>
