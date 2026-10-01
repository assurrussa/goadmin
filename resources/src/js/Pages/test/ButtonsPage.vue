<template>
  <div class="min-h-screen bg-background text-text-primary p-8">
    <div class="max-w-6xl mx-auto">
      <!-- Заголовок -->
      <div class="bg-card rounded-lg shadow-md p-6 mb-8">
        <h1 class="text-3xl font-bold mb-2 text-text-primary">Тестирование кнопок</h1>
        <p class="text-text-secondary">Демонстрация всех стилей кнопок в административной панели</p>
      </div>

      <!-- AppButton базовые варианты -->
      <div class="bg-card rounded-lg shadow-md p-6 mb-8">
        <h2 class="text-2xl font-semibold mb-6 text-text-primary">AppButton варианты</h2>

        <!-- Варианты кнопок -->
        <div class="mb-8">
          <h3 class="text-lg font-medium mb-4 text-text-primary">Варианты</h3>
          <div class="flex flex-wrap gap-4">
            <AppButton variant="primary">Primary</AppButton>
            <AppButton variant="secondary">Secondary</AppButton>
            <AppButton variant="danger">Danger</AppButton>
            <AppButton variant="success">Success</AppButton>
            <AppButton variant="warning">Warning</AppButton>
            <AppButton variant="ghost">Ghost</AppButton>
            <AppButton variant="outline">Outline</AppButton>
          </div>
        </div>

        <!-- Размеры кнопок -->
        <div class="mb-8">
          <h3 class="text-lg font-medium mb-4 text-text-primary">Размеры</h3>
          <div class="flex flex-wrap items-center gap-4">
            <AppButton variant="primary" size="xs">Extra Small</AppButton>
            <AppButton variant="primary" size="sm">Small</AppButton>
            <AppButton variant="primary" size="md">Medium</AppButton>
            <AppButton variant="primary" size="lg">Large</AppButton>
            <AppButton variant="primary" size="xl">Extra Large</AppButton>
          </div>
        </div>

        <!-- Состояния кнопок -->
        <div class="mb-8">
          <h3 class="text-lg font-medium mb-4 text-text-primary">Состояния</h3>
          <div class="flex flex-wrap gap-4">
            <AppButton variant="primary">Normal</AppButton>
            <AppButton variant="primary" disabled>Disabled</AppButton>
            <FormButton variant="primary" :loading="true" loading-text="Loading">
              Loading
            </FormButton>
          </div>
        </div>

        <!-- Модификаторы -->
        <div class="mb-8">
          <h3 class="text-lg font-medium mb-4 text-text-primary">Модификаторы</h3>
          <div class="space-y-4">
            <div>
              <p class="text-sm text-text-secondary mb-2">Полная ширина:</p>
              <AppButton variant="primary" fullWidth>Full Width Button</AppButton>
            </div>
            <div>
              <p class="text-sm text-text-secondary mb-2">Округлые:</p>
              <div class="flex gap-4">
                <AppButton variant="primary" rounded>Rounded Primary</AppButton>
                <AppButton variant="secondary" rounded>Rounded Secondary</AppButton>
              </div>
            </div>
          </div>
        </div>

        <!-- Группы кнопок -->
        <div class="mb-8">
          <h3 class="text-lg font-medium mb-4 text-text-primary">Группы кнопок</h3>
          <div class="inline-flex rounded-md border border-border-primary overflow-hidden">
            <AppButton variant="secondary" class="rounded-none border-0 first:rounded-l-md">
              Первая
            </AppButton>
            <AppButton variant="secondary" class="rounded-none border-0">Вторая</AppButton>
            <AppButton variant="secondary" class="rounded-none border-0 last:rounded-r-md">
              Третья
            </AppButton>
          </div>
        </div>
      </div>

      <!-- FormButton компонент -->
      <div class="bg-card rounded-lg shadow-md p-6 mb-8">
        <h2 class="text-2xl font-semibold mb-6 text-text-primary">FormButton компонент</h2>

        <!-- Варианты FormButton -->
        <div class="mb-8">
          <h3 class="text-lg font-medium mb-4 text-text-primary">Варианты</h3>
          <div class="flex flex-wrap gap-4">
            <FormButton variant="primary">Primary</FormButton>
            <FormButton variant="secondary">Secondary</FormButton>
            <FormButton variant="danger">Danger</FormButton>
            <FormButton variant="success">Success</FormButton>
            <FormButton variant="warning">Warning</FormButton>
            <FormButton variant="ghost">Ghost</FormButton>
            <FormButton variant="outline">Outline</FormButton>
          </div>
        </div>

        <!-- Размеры FormButton -->
        <div class="mb-8">
          <h3 class="text-lg font-medium mb-4 text-text-primary">Размеры</h3>
          <div class="flex flex-wrap items-center gap-4">
            <FormButton variant="primary" size="xs">Extra Small</FormButton>
            <FormButton variant="primary" size="sm">Small</FormButton>
            <FormButton variant="primary" size="md">Medium</FormButton>
            <FormButton variant="primary" size="lg">Large</FormButton>
            <FormButton variant="primary" size="xl">Extra Large</FormButton>
          </div>
        </div>

        <!-- Состояния FormButton -->
        <div class="mb-8">
          <h3 class="text-lg font-medium mb-4 text-text-primary">Состояния</h3>
          <div class="flex flex-wrap gap-4">
            <FormButton variant="primary">Normal</FormButton>
            <FormButton variant="primary" disabled>Disabled</FormButton>
            <FormButton variant="primary" :loading="true" loading-text="Загрузка..."
              >Loading</FormButton
            >
          </div>
        </div>

        <!-- С иконками -->
        <div class="mb-8">
          <h3 class="text-lg font-medium mb-4 text-text-primary">С иконками</h3>
          <div class="flex flex-wrap gap-4">
            <FormButton variant="primary">
              <template #leftIcon>
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M12 6v6m0 0v6m0-6h6m-6 0H6"
                  />
                </svg>
              </template>
              Добавить
            </FormButton>
            <FormButton variant="secondary">
              Сохранить
              <template #rightIcon>
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M5 13l4 4L19 7"
                  />
                </svg>
              </template>
            </FormButton>
          </div>
        </div>
      </div>

      <!-- Сравнение подходов -->
      <div class="bg-card rounded-lg shadow-md p-6 mb-8">
        <h2 class="text-2xl font-semibold mb-6 text-text-primary">Сравнение подходов</h2>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-8">
          <!-- AppButton -->
          <div>
            <h3 class="text-lg font-medium mb-4 text-text-primary">AppButton</h3>
            <div class="space-y-4">
              <div class="flex items-center gap-4">
                <AppButton variant="primary">
                  <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M12 6v6m0 0v6m0-6h6m-6 0H6"
                    />
                  </svg>
                  Создать
                </AppButton>
                <code class="text-sm bg-surface-variant px-2 py-1 rounded"
                  >AppButton variant="primary"</code
                >
              </div>

              <div class="flex items-center gap-4">
                <AppButton variant="danger" size="sm">
                  <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                    />
                  </svg>
                  Удалить
                </AppButton>
                <code class="text-sm bg-surface-variant px-2 py-1 rounded"
                  >AppButton variant="danger" size="sm"</code
                >
              </div>
            </div>

            <div class="mt-4 p-4 bg-surface-variant rounded-lg">
              <h4 class="font-medium text-text-primary mb-2">Плюсы:</h4>
              <ul class="text-sm text-text-secondary space-y-1">
                <li>• Единый API для кнопок</li>
                <li>• Темизация через shadcn tokens</li>
                <li>• Поддержка размеров и иконок</li>
              </ul>
            </div>
          </div>

          <!-- FormButton компонент -->
          <div>
            <h3 class="text-lg font-medium mb-4 text-text-primary">FormButton компонент</h3>
            <div class="space-y-4">
              <div class="flex items-center gap-4">
                <FormButton variant="primary">
                  <template #leftIcon>
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M12 6v6m0 0v6m0-6h6m-6 0H6"
                      />
                    </svg>
                  </template>
                  Создать
                </FormButton>
                <code class="text-sm bg-surface-variant px-2 py-1 rounded"
                  >FormButton variant="primary"</code
                >
              </div>

              <div class="flex items-center gap-4">
                <FormButton variant="danger" size="sm">
                  <template #leftIcon>
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                      />
                    </svg>
                  </template>
                  Удалить
                </FormButton>
                <code class="text-sm bg-surface-variant px-2 py-1 rounded"
                  >FormButton variant="danger" size="sm"</code
                >
              </div>
            </div>

            <div class="mt-4 p-4 bg-surface-variant rounded-lg">
              <h4 class="font-medium text-text-primary mb-2">Плюсы:</h4>
              <ul class="text-sm text-text-secondary space-y-1">
                <li>• TypeScript типизация</li>
                <li>• Встроенные состояния загрузки</li>
                <li>• Слоты для иконок</li>
                <li>• Единообразие в формах</li>
              </ul>
            </div>
          </div>
        </div>
      </div>

      <!-- Рекомендации -->
      <div class="bg-card rounded-lg shadow-md p-6">
        <h2 class="text-2xl font-semibold mb-6 text-text-primary">Рекомендации по использованию</h2>

        <div class="space-y-6">
          <div class="p-4 bg-surface-variant border border-success rounded-lg">
            <h3 class="font-medium text-success mb-2">✅ Используйте FormButton для:</h3>
            <ul class="text-sm text-text-secondary space-y-1">
              <li>• Форм (отправка, сброс, валидация)</li>
              <li>• Действий с состоянием загрузки</li>
              <li>• Кнопок с иконками</li>
              <li>• Компонентов, требующих типизации</li>
            </ul>
          </div>

          <div class="p-4 bg-surface-variant border border-info rounded-lg">
            <h3 class="font-medium text-info mb-2">ℹ️ Используйте AppButton для:</h3>
            <ul class="text-sm text-text-secondary space-y-1">
              <li>• Простых кнопок без сложной логики</li>
              <li>• Быстрого прототипирования</li>
              <li>• Кнопок в списках и таблицах</li>
              <li>• Кастомных компонентов</li>
            </ul>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import FormButton from '@/components/form/FormButton.vue'
import AppButton from '@/components/ui/AppButton.vue'

defineOptions({
  name: 'ButtonsTest',
})
</script>
