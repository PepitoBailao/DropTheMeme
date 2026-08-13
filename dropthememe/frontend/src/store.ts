import { load } from '@tauri-apps/plugin-store'

const STORE_PATH = 'dropthememe.json'

export interface UserInfo {
  userId: string
  steamId: string
  username: string
  avatar: string
  token: string
}

let store: Awaited<ReturnType<typeof load>> | null = null

async function getStore() {
  if (!store) {
    store = await load(STORE_PATH, { autoSave: true })
  }
  return store
}

export async function saveUser(user: UserInfo) {
  const s = await getStore()
  await s.set('user', user)
}

export async function getUser(): Promise<UserInfo | null> {
  const s = await getStore()
  const user = await s.get<UserInfo>('user')
  return user ?? null
}

export async function clearUser() {
  const s = await getStore()
  await s.delete('user')
}

export async function getToken(): Promise<string | null> {
  const user = await getUser()
  return user?.token ?? null
}
