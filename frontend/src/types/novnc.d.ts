declare module '@novnc/novnc' {
  export default class RFB {
    constructor(target: HTMLElement, url: string, options?: Record<string, unknown>)
    scaleViewport: boolean
    resizeSession: boolean
    viewOnly: boolean
    clipViewport: boolean
    showDotCursor: boolean
    background: string
    qualityLevel: number
    compressionLevel: number
    disconnect(): void
    focus(): void
    sendCtrlAltDel(): void
    sendCredentials(credentials: { password?: string; username?: string; target?: string }): void
    addEventListener(type: string, listener: (e: CustomEvent<Record<string, unknown>>) => void): void
    removeEventListener(type: string, listener: (e: CustomEvent<Record<string, unknown>>) => void): void
  }
}
