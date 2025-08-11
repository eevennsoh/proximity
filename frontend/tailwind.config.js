/** @type {import('tailwindcss').Config} */
module.exports = {
  darkMode: ["class"],
  content: ["./index.html", "./src/**/*.{js,jsx,ts,tsx}"],
  theme: {
    extend: {
      colors: {
        brand: {
          50: "#f0f7ff",
          100: "#dfeeff",
          200: "#b9dbff",
          300: "#8bc2ff",
          400: "#5aa5ff",
          500: "#2d88ff",
          600: "#126de6",
          700: "#0a57b8",
          800: "#0a4793",
          900: "#0c3c78"
        },
        accent: {
          300: "#ff80bf",
          400: "#ff4d9d",
          500: "#ff1a75",
          600: "#e60061"
        }
      },
      fontFamily: {
        sans: ["\"Nunito\"", "ui-sans-serif", "system-ui", "-apple-system", "Segoe UI", "Roboto", "Helvetica Neue", "Arial", "Noto Sans", "Apple Color Emoji", "Segoe UI Emoji", "Segoe UI Symbol"]
      },
      boxShadow: {
        card: "0 10px 25px -10px rgba(0,0,0,0.25)",
        glow: "0 0 30px -10px rgba(45,136,255,0.5), 0 0 50px -20px rgba(236,72,153,0.5)"
      },
      backgroundImage: {
        "radial-faded": "radial-gradient(800px 600px at 0% 0%, rgba(45,136,255,0.12), transparent 60%), radial-gradient(800px 600px at 100% 100%, rgba(236,72,153,0.12), transparent 60%)"
      },
      keyframes: {
        shimmer: {
          "0%": { transform: "translateX(-100%)" },
          "100%": { transform: "translateX(100%)" }
        },
        float: {
          "0%, 100%": { transform: "translateY(0px)" },
          "50%": { transform: "translateY(-6px)" }
        },
        pulseGlow: {
          "0%, 100%": { boxShadow: "0 0 0 0 rgba(45,136,255,0.5)" },
          "50%": { boxShadow: "0 0 20px 0 rgba(236,72,153,0.5)" }
        }
      },
      animation: {
        shimmer: "shimmer 2s linear infinite",
        float: "float 6s ease-in-out infinite",
        "pulse-glow": "pulseGlow 2.5s ease-in-out infinite"
      }
    }
  },
  plugins: [require("tailwindcss-animate")]
};
