import { Component, type ReactNode } from "react"

import { PanelErrorState } from "@/panels/panel-error"
import { RendererLoadError } from "@/panels/renderer-load"

type State = { failed: boolean; error: unknown }

export class EditorBoundary extends Component<{ children: ReactNode }, State> {
  state: State = { failed: false, error: undefined }

  static getDerivedStateFromError(error: unknown): State {
    return { failed: true, error }
  }

  retry = () => {
    if (this.state.error instanceof RendererLoadError) location.reload()
    else this.setState({ failed: false, error: undefined })
  }

  render() {
    if (!this.state.failed) return this.props.children
    return (
      <PanelErrorState
        title="Could not load the panel editor"
        error={this.state.error}
        onRetry={this.retry}
        retryLabel={
          this.state.error instanceof RendererLoadError
            ? "Reload page"
            : "Retry"
        }
      />
    )
  }
}
