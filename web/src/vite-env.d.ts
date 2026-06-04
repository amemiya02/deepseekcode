/// <reference types="vite/client" />

// `global` is the Node-style alias for `globalThis`, used by Vitest tests running
// in the jsdom environment (e.g. workspace.test.ts stubs `global.fetch`). We don't
// depend on @types/node, so declare the alias here for the type-checker.
declare const global: typeof globalThis
