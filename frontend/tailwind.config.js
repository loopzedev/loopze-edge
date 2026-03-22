/** @type {import('tailwindcss').Config} */
export default {
  content: [
    './index.html',
    './src/**/*.{vue,js,ts,jsx,tsx}',
  ],
  theme: {
    extend: {
      colors: {
        accent: {
          DEFAULT: '#58a6ff',
          dim: '#388bfd',
          muted: '#58a6ff33',
          subtle: '#58a6ff0d',
        },
        terminal: {
          bg:           '#0d1117',
          surface:      '#161b22',
          'surface-alt':'#1c2128',
          border:       '#30363d',
          'border-light':'#3d444d',
          text:         '#e6edf3',
          'text-dim':   '#7d8590',
          'text-bright':'#f0f6fc',
        },
        status: {
          success: '#3fb950',
          warning: '#d29922',
          error:   '#f85149',
        },
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'sans-serif'],
        mono: ['IBM Plex Mono', 'Consolas', 'Monaco', 'Courier New', 'monospace'],
      },
      borderRadius: {
        DEFAULT: '3px', none: '0px', sm: '2px', md: '4px',
        lg: '6px', xl: '8px', '2xl': '12px', '3xl': '16px',
        full: '9999px',
      },
    },
  },
  plugins: [],
};
