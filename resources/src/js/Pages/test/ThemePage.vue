<template>
  <div class="min-h-screen bg-background text-text-primary p-8">
    <div class="max-w-6xl mx-auto">
      <div class="bg-card rounded-lg shadow-md p-6 mb-8">
        <h1 class="text-3xl font-bold mb-2 text-text-primary">Тестирование темы</h1>
        <p class="text-text-secondary">Проверка переключения между светлой и темной темой</p>
      </div>

      <!-- Цветовые демонстрации -->
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6 mb-8">
        <!-- Основные цвета -->
        <div class="bg-card rounded-lg shadow-md p-6 border border-border-primary">
          <h3 class="font-semibold mb-4 text-text-primary">Основные цвета</h3>
          <div class="space-y-3">
            <div class="bg-primary text-primary-contrast p-3 rounded">Primary</div>
            <div class="bg-primary-light text-primary-contrast p-3 rounded">Primary Light</div>
            <div class="bg-primary-dark text-primary-contrast p-3 rounded">Primary Dark</div>
          </div>
        </div>

        <!-- Поверхности -->
        <div class="bg-card rounded-lg shadow-md p-6 border border-border-primary">
          <h3 class="font-semibold mb-4 text-text-primary">Поверхности</h3>
          <div class="space-y-3">
            <div class="bg-background border border-border-primary p-3 rounded text-text-primary">
              Background
            </div>
            <div class="bg-surface border border-border-primary p-3 rounded text-text-primary">
              Surface
            </div>
            <div
              class="bg-surface-variant border border-border-primary p-3 rounded text-text-primary"
            >
              Surface Variant
            </div>
          </div>
        </div>

        <!-- Текст -->
        <div class="bg-card rounded-lg shadow-md p-6 border border-border-primary">
          <h3 class="font-semibold mb-4 text-text-primary">Текст</h3>
          <div class="space-y-2">
            <p class="text-text-primary">Primary Text</p>
            <p class="text-text-secondary">Secondary Text</p>
            <p class="text-text-tertiary">Tertiary Text</p>
            <p class="text-text-disabled">Disabled Text</p>
          </div>
        </div>

        <!-- Состояния -->
        <div class="bg-card rounded-lg shadow-md p-6 border border-border-primary">
          <h3 class="font-semibold mb-4 text-text-primary">Состояния</h3>
          <div class="space-y-3">
            <div class="bg-success text-white p-3 rounded">Success</div>
            <div class="bg-warning text-white p-3 rounded">Warning</div>
            <div class="bg-error text-white p-3 rounded">Error</div>
            <div class="bg-info text-white p-3 rounded">Info</div>
          </div>
        </div>

        <!-- Интерактивные элементы -->
        <div class="bg-card rounded-lg shadow-md p-6 border border-border-primary">
          <h3 class="font-semibold mb-4 text-text-primary">Элементы</h3>
          <div class="space-y-3">
            <AppButton variant="primary" fullWidth>Кнопка</AppButton>
            <AppInput type="text" placeholder="Поле ввода" />
            <div class="bg-surface-variant p-3 rounded border border-border-primary">
              <p class="text-text-secondary">Карточка контента</p>
            </div>
          </div>
        </div>

        <!-- Отладочная информация -->
        <div class="bg-card rounded-lg shadow-md p-6 border border-border-primary">
          <h3 class="font-semibold mb-4 text-text-primary">Отладка</h3>
          <div class="space-y-2 text-sm font-mono">
            <div class="text-text-secondary">
              Тема: <span class="font-bold text-text-primary">{{ themeStore.theme }}</span>
            </div>
            <div class="text-text-secondary">
              Темная:
              <span class="font-bold text-text-primary">{{
                themeStore.isDark ? 'Да' : 'Нет'
              }}</span>
            </div>
            <div class="text-text-secondary">
              HTML: <span class="font-bold text-text-primary">{{ htmlClasses }}</span>
            </div>
            <div class="text-text-secondary">
              localStorage:
              <span class="font-bold text-text-primary">{{ savedTheme || 'не сохранено' }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Кнопки управления -->
      <div class="bg-card rounded-lg shadow-md p-6 border border-border-primary">
        <h3 class="font-semibold mb-4 text-text-primary">Управление темой</h3>
        <div class="flex flex-wrap gap-4">
          <AppButton variant="primary" @click="toggleTheme">Переключить тему</AppButton>

          <AppButton variant="secondary" @click="setLight">Светлая тема</AppButton>

          <AppButton variant="secondary" @click="setDark">Темная тема</AppButton>

          <AppButton variant="success" @click="refreshDebugInfo">Обновить отладку</AppButton>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useThemeStore } from '@/stores/theme'
import AppButton from '@/components/ui/AppButton.vue'
import AppInput from '@/components/ui/AppInput.vue'

defineOptions({
  name: 'ThemeTest',
})

const themeStore = useThemeStore()

// Реактивные переменные для отладки
const htmlClasses = ref('')
const savedTheme = ref('')

// Методы
const toggleTheme = () => {
  themeStore.toggle()
  refreshDebugInfo()
}

const setLight = () => {
  themeStore.setTheme('light')
  refreshDebugInfo()
}

const setDark = () => {
  themeStore.setTheme('dark')
  refreshDebugInfo()
}

const refreshDebugInfo = () => {
  htmlClasses.value = document.documentElement.className
  savedTheme.value = localStorage.getItem('theme') || ''
}

// Инициализация
onMounted(() => {
  refreshDebugInfo()

  // Обновляем отладочную информацию каждые 500мс
  setInterval(refreshDebugInfo, 500)
})
</script>
