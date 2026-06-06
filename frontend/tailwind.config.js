/** @type {import('tailwindcss').Config} */
const withOpacity = (variable) => `rgb(var(${variable}) / <alpha-value>)`

export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // Semantic tokens backed by CSS variables in src/style.css.
        surface: {
          page: withOpacity('--color-surface-page'),
          card: withOpacity('--color-surface-card'),
          secondary: withOpacity('--color-surface-secondary'),
          hover: withOpacity('--color-surface-hover')
        },
        content: {
          primary: withOpacity('--color-content-primary'),
          secondary: withOpacity('--color-content-secondary'),
          tertiary: withOpacity('--color-content-tertiary'),
          inverse: withOpacity('--color-content-inverse')
        },
        stroke: {
          subtle: withOpacity('--color-stroke-subtle'),
          default: withOpacity('--color-stroke-default'),
          DEFAULT: withOpacity('--color-stroke-default'),
          strong: withOpacity('--color-stroke-strong'),
          brand: withOpacity('--color-stroke-brand')
        },
        status: {
          success: withOpacity('--color-status-success'),
          warning: withOpacity('--color-status-warning'),
          error: withOpacity('--color-status-error'),
          info: withOpacity('--color-status-info')
        },
        // 主色 / 品牌色 - 暖棕色系，由 CSS 变量在 Light/Dark 间切换
        primary: {
          50: withOpacity('--color-primary-50'),
          100: withOpacity('--color-primary-100'),
          200: withOpacity('--color-primary-200'),
          300: withOpacity('--color-primary-300'),
          400: withOpacity('--color-primary-400'),
          500: withOpacity('--color-primary-500'),
          600: withOpacity('--color-primary-600'),
          700: withOpacity('--color-primary-700'),
          800: withOpacity('--color-primary-800'),
          900: withOpacity('--color-primary-900'),
          950: withOpacity('--color-primary-950')
        },
        // 中性色 - Light Mode 设计稿色阶，兼容既有 gray-* utility
        gray: {
          50: withOpacity('--color-gray-50'),
          100: withOpacity('--color-gray-100'),
          200: withOpacity('--color-gray-200'),
          300: withOpacity('--color-gray-300'),
          400: withOpacity('--color-gray-400'),
          500: withOpacity('--color-gray-500'),
          600: withOpacity('--color-gray-600'),
          700: withOpacity('--color-gray-700'),
          800: withOpacity('--color-gray-800'),
          900: withOpacity('--color-gray-900'),
          950: withOpacity('--color-gray-950')
        },
        // 辅助色沿用品牌色，兼容既有 text-gradient 等用法
        accent: {
          50: withOpacity('--color-primary-50'),
          100: withOpacity('--color-primary-100'),
          200: withOpacity('--color-primary-200'),
          300: withOpacity('--color-primary-300'),
          400: withOpacity('--color-primary-400'),
          500: withOpacity('--color-primary-500'),
          600: withOpacity('--color-primary-600'),
          700: withOpacity('--color-primary-700'),
          800: withOpacity('--color-primary-800'),
          900: withOpacity('--color-primary-900'),
          950: withOpacity('--color-primary-950')
        },
        // 深色模式背景 / 中性色，兼容既有 dark:* utility
        dark: {
          50: withOpacity('--color-dark-50'),
          100: withOpacity('--color-dark-100'),
          200: withOpacity('--color-dark-200'),
          300: withOpacity('--color-dark-300'),
          400: withOpacity('--color-dark-400'),
          500: withOpacity('--color-dark-500'),
          600: withOpacity('--color-dark-600'),
          700: withOpacity('--color-dark-700'),
          800: withOpacity('--color-dark-800'),
          900: withOpacity('--color-dark-900'),
          950: withOpacity('--color-dark-950')
        }
      },
      fontFamily: {
        sans: [
          'system-ui',
          '-apple-system',
          'BlinkMacSystemFont',
          'Segoe UI',
          'Roboto',
          'Helvetica Neue',
          'Arial',
          'PingFang SC',
          'Hiragino Sans GB',
          'Microsoft YaHei',
          'sans-serif'
        ],
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace']
      },
      boxShadow: {
        glass: '0 8px 32px rgba(33, 22, 13, 0.08)',
        'glass-sm': '0 4px 16px rgba(33, 22, 13, 0.06)',
        glow: '0 0 20px rgb(var(--color-primary-500) / 0.25)',
        'glow-lg': '0 0 40px rgb(var(--color-primary-500) / 0.35)',
        card: '0 1px 3px rgb(var(--shadow-color) / 0.04), 0 1px 2px rgb(var(--shadow-color) / 0.06)',
        'card-hover': '0 10px 40px rgb(var(--shadow-color) / 0.08)',
        'inner-glow': 'inset 0 1px 0 rgba(255, 255, 255, 0.1)'
      },
      backgroundImage: {
        'gradient-radial': 'radial-gradient(var(--tw-gradient-stops))',
        'gradient-primary':
          'linear-gradient(135deg, rgb(var(--color-primary-500)) 0%, rgb(var(--color-primary-600)) 100%)',
        'gradient-dark':
          'linear-gradient(135deg, rgb(var(--color-surface-card)) 0%, rgb(var(--color-surface-secondary)) 100%)',
        'gradient-glass':
          'linear-gradient(135deg, rgba(255,255,255,0.1) 0%, rgba(255,255,255,0.05) 100%)',
        'mesh-gradient':
          'radial-gradient(at 40% 20%, rgb(var(--color-primary-300) / 0.16) 0px, transparent 50%), radial-gradient(at 80% 0%, rgb(var(--color-primary-500) / 0.1) 0px, transparent 50%), radial-gradient(at 0% 50%, rgb(var(--color-primary-400) / 0.1) 0px, transparent 50%)'
      },
      animation: {
        'fade-in': 'fadeIn 0.3s ease-out',
        'slide-up': 'slideUp 0.3s ease-out',
        'slide-down': 'slideDown 0.3s ease-out',
        'slide-in-right': 'slideInRight 0.3s ease-out',
        'scale-in': 'scaleIn 0.2s ease-out',
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        shimmer: 'shimmer 2s linear infinite',
        glow: 'glow 2s ease-in-out infinite alternate'
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' }
        },
        slideUp: {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideDown: {
          '0%': { opacity: '0', transform: 'translateY(-10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideInRight: {
          '0%': { opacity: '0', transform: 'translateX(20px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' }
        },
        scaleIn: {
          '0%': { opacity: '0', transform: 'scale(0.95)' },
          '100%': { opacity: '1', transform: 'scale(1)' }
        },
        shimmer: {
          '0%': { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0' }
        },
        glow: {
          '0%': { boxShadow: '0 0 20px rgb(var(--color-primary-500) / 0.25)' },
          '100%': { boxShadow: '0 0 30px rgb(var(--color-primary-500) / 0.4)' }
        }
      },
      backdropBlur: {
        xs: '2px'
      },
      borderRadius: {
        '4xl': '2rem'
      }
    }
  },
  plugins: []
}
