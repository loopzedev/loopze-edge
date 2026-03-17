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
        },
        terminal: {
          bg:           '#0d1117',
          surface:      '#161b22',
          border:       '#30363d',
          text:         '#e6edf3',
          'text-dim':   '#7d8590',
          'text-bright':'#f0f6fc',
        },
      },
      fontFamily: {
        mono: ['Consolas', 'Monaco', 'Courier New', 'monospace'],
      },
      borderRadius: {
        DEFAULT: '0px', none: '0px', sm: '0px', md: '0px',
        lg: '0px', xl: '0px', '2xl': '0px', '3xl': '0px',
        full: '9999px',
      },
    },
  },
  plugins: [],
};
