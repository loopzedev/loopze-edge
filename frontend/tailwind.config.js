/** @type {import('tailwindcss').Config} */
export default {
  content: [
    './index.html',
    './src/**/*.{vue,js,ts,jsx,tsx}',
  ],
  theme: {
    extend: {
      colors: {
        amber: {
          DEFAULT: '#FFBF00',
          dim: '#CC9900',
        },
        terminal: {
          bg: '#1a1a0e',
          surface: '#252518',
          border: '#3a3a28',
          text: '#FFBF00',
          'text-dim': '#998a00',
          'text-bright': '#FFD966',
        },
      },
      fontFamily: {
        mono: ['Consolas', 'Monaco', 'Courier New', 'monospace'],
      },
      borderRadius: {
        DEFAULT: '0px',
        none: '0px',
        sm: '0px',
        md: '0px',
        lg: '0px',
        xl: '0px',
        '2xl': '0px',
        '3xl': '0px',
        full: '0px',
      },
    },
  },
  plugins: [],
};
