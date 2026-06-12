import {
  defineConfig,
  minimal2023Preset,
} from "@vite-pwa/assets-generator/config";

// Brand purple from favicon.svg — iOS renders transparency as black and the
// default preset pads with white, so force full-bleed brand background.
const background = "#6C3AED";

export default defineConfig({
  preset: {
    ...minimal2023Preset,
    maskable: {
      ...minimal2023Preset.maskable,
      resizeOptions: { background },
    },
    apple: {
      ...minimal2023Preset.apple,
      resizeOptions: { background },
    },
  },
  images: ["public/favicon.svg"],
});
