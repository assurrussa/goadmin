<template>
  <div class="space-y-6">
    <h1 class="text-2xl font-semibold text-text-primary">Компоненты и состояния</h1>
    <Card class="p-6 space-y-4">
      <h2 class="text-lg font-semibold">Вкладки</h2>
      <Tabs default-value="details">
        <TabsList aria-label="Разделы карточки">
          <TabsTrigger id="design-tab-details" value="details">Основное</TabsTrigger>
          <TabsTrigger id="design-tab-history" value="history">История</TabsTrigger>
          <TabsTrigger value="disabled" disabled>Недоступно</TabsTrigger>
        </TabsList>
        <TabsContent value="details">Данные пользователя и настройки доступа.</TabsContent>
        <TabsContent value="history">Изменения профиля и последние действия.</TabsContent>
      </Tabs>
    </Card>
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
        <AppButton id="design-secondary" variant="secondary">Вторичное</AppButton>
        <AppButton id="design-outline" variant="outline">Контурное</AppButton>
        <AppButton id="design-ghost" variant="ghost">Прозрачное</AppButton>
        <AppButton id="design-link" variant="link">Ссылка</AppButton>
      </div>
      <div class="flex flex-wrap gap-2">
        <AppButton id="design-loading" loading>Обработка</AppButton>
        <AppButton id="design-button-disabled" disabled>Недоступно</AppButton>
        <AppButton id="design-as-child" as-child variant="outline"
          ><a href="#design-name">К полю имени</a></AppButton
        >
        <AppButton id="design-as-child-disabled" as-child disabled
          ><a href="#design-name">Недоступная ссылка</a></AppButton
        >
        <AppButton id="design-as-child-loading" as-child loading
          ><a href="#design-name">Ссылка загружается</a></AppButton
        >
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
      <FormInput
        id="design-readonly-invalid"
        label="Только чтение с ошибкой"
        model-value="Недопустимое значение"
        readonly
        error="Значение требует проверки"
      />
      <FormSelect
        id="design-select"
        v-model="selection"
        label="Раздел"
        :options="selectOptions"
        placeholder="Выберите раздел"
      />
      <FormSelect
        id="design-select-error"
        label="Раздел с ошибкой"
        :options="selectOptions"
        error="Выберите доступный раздел"
      />
      <FormSelect
        id="design-select-disabled"
        label="Недоступный выбор"
        model-value="overview"
        :options="selectOptions"
        disabled
      />
      <FormCheckbox
        id="design-checkbox"
        v-model="checked"
        label="Получать обновления (только состояние страницы)"
      />
      <FormCheckbox
        id="design-checkbox-disabled"
        :model-value="true"
        label="Недоступный флажок"
        disabled
      />
      <FormCheckbox
        id="design-checkbox-error"
        :model-value="false"
        label="Обязательное подтверждение"
        error="Нужно подтверждение"
      />
      <div style="--color-error: rgb(126, 34, 206)">
        <FormInput id="design-host-error" label="Цвет хоста" error="Ошибка с цветом хоста" />
      </div>
      <AppButton id="design-override" class="h-16 rounded-none">Переопределённый размер</AppButton>
      <DropdownMenu>
        <DropdownMenuTrigger as-child
          ><AppButton id="design-dropdown" variant="outline"
            >Состояния меню</AppButton
          ></DropdownMenuTrigger
        >
        <DropdownMenuContent>
          <DropdownMenuLabel>Только локальная демонстрация</DropdownMenuLabel>
          <DropdownMenuItem @select="lastAction = 'Выбран обычный пункт'"
            >Обычный пункт</DropdownMenuItem
          >
          <DropdownMenuItem disabled>Недоступный пункт</DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuCheckboxItem v-model="dropdownChecked"
            >Показывать сведения</DropdownMenuCheckboxItem
          >
          <DropdownMenuRadioGroup v-model="density">
            <DropdownMenuRadioItem value="comfortable">Обычная плотность</DropdownMenuRadioItem>
            <DropdownMenuRadioItem value="compact">Компактная плотность</DropdownMenuRadioItem>
          </DropdownMenuRadioGroup>
          <DropdownMenuSub>
            <DropdownMenuSubTrigger>Вложенное меню</DropdownMenuSubTrigger>
            <DropdownMenuSubContent>
              <DropdownMenuItem @select="lastAction = 'Выбран вложенный пункт'"
                >Вложенный пункт</DropdownMenuItem
              >
              <DropdownMenuItem disabled>Недоступный вложенный пункт</DropdownMenuItem>
            </DropdownMenuSubContent>
          </DropdownMenuSub>
        </DropdownMenuContent>
      </DropdownMenu>
      <p v-if="lastAction" role="status" class="text-sm text-text-secondary">{{ lastAction }}</p>
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
    <Card class="p-6 space-y-4">
      <h2 class="text-lg font-semibold">Изображение</h2>
      <AppButton id="design-open-cropper" @click="showCropper = true"
        >Обрезать изображение</AppButton
      >
      <p v-if="cropResult" role="status">{{ cropResult }}</p>
      <img
        v-if="croppedUrl"
        :src="croppedUrl"
        alt="Результат обрезки"
        class="max-w-full rounded-lg"
      />
    </Card>
    <section class="space-y-4" aria-label="Редактор текста">
      <h2 class="text-lg font-semibold">Редактор текста</h2>
      <RichTextEditor v-model="document" :allow-file-upload="false" />
    </section>
    <ImageCropper
      v-if="showCropper"
      :image-url="trainingImage"
      file-name="training.png"
      :max-width="640"
      :max-height="640"
      @close="showCropper = false"
      @crop="onCrop"
    />
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppStatus from '@/components/ui/AppStatus.vue'
import FormInput from '@/components/form/FormInput.vue'
import FormSelect from '@/components/form/FormSelect.vue'
import FormCheckbox from '@/components/form/FormCheckbox.vue'
import RichTextEditor from '@/components/form/RichTextEditor.vue'
import ImageCropper from '@/components/imageCropper/ImageCropper.vue'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import trainingImage from '../../demo/sports-club/training.svg?url'
import { Card } from '@/components/ui/card'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuCheckboxItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSub,
  DropdownMenuSubTrigger,
  DropdownMenuSubContent,
} from '@/components/ui/dropdown-menu'
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
const selection = ref<string | number | bigint | null>('overview')
const selectOptions = [
  { value: 'overview', label: 'Обзор', group: 'Рабочие разделы' },
  { value: 'settings', label: 'Настройки', group: 'Рабочие разделы' },
  { value: 'unavailable', label: 'Недоступно', disabled: true, group: 'Другие разделы' },
]
const checked = ref(true)
const dropdownChecked = ref(true)
const density = ref('comfortable')
const lastAction = ref('')
const document = ref('<p>Описание пользователя и рабочие заметки.</p>')
const showCropper = ref(false)
const croppedUrl = ref('')
const cropResult = ref('')
function onCrop(file: File) {
  if (croppedUrl.value) URL.revokeObjectURL(croppedUrl.value)
  croppedUrl.value = URL.createObjectURL(file)
  cropResult.value = `Готово: ${file.name}, ${file.type}, ${file.size} байт`
  showCropper.value = false
}
onBeforeUnmount(() => {
  if (croppedUrl.value) URL.revokeObjectURL(croppedUrl.value)
})
</script>
