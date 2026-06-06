import { create } from 'zustand'

export type ToastKind = 'info' | 'success' | 'warning' | 'danger'

export interface Toast {
  id: number
  kind: ToastKind
  message: string
  durationMs: number
}

export interface ToastInput {
  kind: ToastKind
  message: string
  durationMs?: number
}

interface ToastStore {
  items: Toast[]
}

let seq = 0
export const useToastStore = create<ToastStore>(() => ({ items: [] }))

export function pushToast(input: ToastInput): number {
  const id = ++seq
  const durationMs = input.durationMs ?? 4000
  const toast: Toast = { id, kind: input.kind, message: input.message, durationMs }
  useToastStore.setState((s) => ({ items: [...s.items, toast] }))
  if (durationMs > 0 && durationMs < 100000) {
    setTimeout(() => dismissToast(id), durationMs)
  }
  return id
}

export function dismissToast(id: number): void {
  useToastStore.setState((s) => ({ items: s.items.filter((t) => t.id !== id) }))
}

export function clearToasts(): void {
  useToastStore.setState({ items: [] })
}

// Hook for the renderer to subscribe to the live toast list.
export function useToasts(): Toast[] {
  return useToastStore((s) => s.items)
}
