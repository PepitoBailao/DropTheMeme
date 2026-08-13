import { getToken } from './store'

const API_URL = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'
const WS_URL = import.meta.env.VITE_WS_URL ?? 'ws://localhost:8080'

// ── HTTP ──────────────────────────────────────────────────

async function apiFetch<T>(path: string, options?: RequestInit): Promise<T> {
  const token = await getToken()
  const res = await fetch(`${API_URL}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...options?.headers,
    },
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Error(`API ${res.status}: ${text}`)
  }
  return res.json()
}

export interface AuthResponse {
  token: string
  user_id: string
  username: string
  avatar: string
}

// Auth via ticket Steam (généré par steamworks-rs côté Rust/Tauri)
export async function authSteam(ticket: string): Promise<AuthResponse> {
  return apiFetch<AuthResponse>('/auth/steam', {
    method: 'POST',
    body: JSON.stringify({ ticket }),
  })
}

export interface Group {
  id: string
  name: string
  invite_code: string
}

export async function getMyGroups(): Promise<Group[]> {
  return apiFetch<Group[]>('/groups')
}

export async function createGroup(name: string): Promise<{ group_id: string; invite_code: string }> {
  return apiFetch('/groups', {
    method: 'POST',
    body: JSON.stringify({ name }),
  })
}

export async function joinGroup(inviteCode: string): Promise<{ group_id: string; name: string }> {
  return apiFetch('/groups/join', {
    method: 'POST',
    body: JSON.stringify({ invite_code: inviteCode }),
  })
}

export interface Member {
  user_id: string
  username: string
  avatar: string
}

export async function getGroupMembers(groupId: string): Promise<Member[]> {
  return apiFetch<Member[]>(`/groups/${groupId}/members`)
}

// ── WEBSOCKET ─────────────────────────────────────────────

export interface WSMessage {
  type: 'text' | 'image' | 'audio' | 'video' | 'gif' | 'join' | 'leave' | 'members'
  group_id: string
  user_id: string
  username: string
  avatar: string
  content: string
  duration?: number   // secondes, pour audio/vidéo
  timestamp: number
}

export type WSHandler = (msg: WSMessage) => void

export class GroupSocket {
  private ws: WebSocket | null = null
  private groupId: string
  private handlers: WSHandler[] = []
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null

  constructor(groupId: string) {
    this.groupId = groupId
  }

  async connect() {
    const token = await getToken()
    if (!token) throw new Error('Non authentifié')

    const url = `${WS_URL}/ws?group_id=${this.groupId}`
    this.ws = new WebSocket(url, ['Bearer', token])

    this.ws.onopen = () => {
      console.log(`✅ WebSocket connecté au groupe ${this.groupId}`)
      if (this.reconnectTimer) {
        clearTimeout(this.reconnectTimer)
        this.reconnectTimer = null
      }
    }

    this.ws.onmessage = (event) => {
      try {
        const msg: WSMessage = JSON.parse(event.data)
        this.handlers.forEach(h => h(msg))
      } catch (e) {
        console.error('Message WS invalide', e)
      }
    }

    this.ws.onclose = () => {
      console.log('WebSocket fermé, reconnexion dans 3s...')
      this.reconnectTimer = setTimeout(() => this.connect(), 3000)
    }

    this.ws.onerror = (e) => {
      console.error('Erreur WebSocket', e)
    }
  }

  onMessage(handler: WSHandler) {
    this.handlers.push(handler)
    return () => {
      this.handlers = this.handlers.filter(h => h !== handler)
    }
  }

  send(msg: Partial<WSMessage>) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(msg))
    }
  }

  sendText(content: string) {
    this.send({ type: 'text', content })
  }

  // Pour les médias : on envoie l'URL R2 après upload
  sendMedia(type: WSMessage['type'], url: string, duration?: number) {
    this.send({ type, content: url, duration })
  }

  disconnect() {
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer)
    this.ws?.close()
    this.ws = null
  }
}
