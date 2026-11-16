/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** The first 12 characters of the commit the frontend was built from. */
  readonly VITE_BUILD_SHA: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
