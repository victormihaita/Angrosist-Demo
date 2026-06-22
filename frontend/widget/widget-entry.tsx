import { createRoot } from 'react-dom/client'
import { WidgetApp } from './WidgetApp'
import type { ChatIntent, ChatVertical } from '@/lib/api'

interface WidgetConfig {
  apiUrl?: string
  containerId?: string
  /** Flow vertical — defaults to 'angrosist' (existing embeds unaffected). */
  vertical?: ChatVertical
  /** Flow intent — defaults to 'buy'. Use 'sell' for the PalletClearance seller flow. */
  intent?: ChatIntent
}

let mounted = false

function init(config: WidgetConfig = {}) {
  if (mounted) return
  mounted = true

  // Override global API URL for the widget
  if (config.apiUrl) {
    ;(window as unknown as Record<string, unknown>).__ANGROSIST_API_URL__ = config.apiUrl
  }

  // Find or create container
  let container: HTMLElement | null = null
  if (config.containerId) {
    container = document.getElementById(config.containerId)
  }

  if (!container) {
    // Floating button + panel mode
    const wrapper = document.createElement('div')
    wrapper.id = '__angrosist_widget__'
    wrapper.style.cssText =
      'position:fixed;bottom:24px;right:24px;z-index:999999;display:flex;flex-direction:column;align-items:flex-end;gap:12px;'
    document.body.appendChild(wrapper)
    container = wrapper
  }

  const root = createRoot(container)

  function render(open: boolean) {
    root.render(
      open ? (
        <WidgetApp
          apiUrl={config.apiUrl}
          vertical={config.vertical}
          intent={config.intent}
          onClose={() => render(false)}
        />
      ) : (
        <button
          onClick={() => render(true)}
          style={{
            width: '56px', height: '56px', borderRadius: '50%',
            background: '#111827', color: '#fff', border: 'none',
            cursor: 'pointer', fontSize: '24px', boxShadow: '0 4px 16px rgba(0,0,0,0.2)',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
          }}
          title="Chat cu Euro Intermed"
        >
          💬
        </button>
      ),
    )
  }

  render(false)
}

;(window as unknown as Record<string, unknown>).AngrosistChat = { init }
