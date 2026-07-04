import axios from 'axios'

// Shared axios instance. Session auth rides on an HttpOnly cookie, so we send
// credentials with every request. baseURL '/api' matches the Go backend and the
// Vite dev proxy.
const api = axios.create({
  baseURL: '/api',
  withCredentials: true,
})

// Normalize the backend's {error:{code,message}} envelope into err.message.
api.interceptors.response.use(
  (r) => r,
  (err) => {
    const data = err?.response?.data
    if (data?.error?.message) err.message = data.error.message
    else if (typeof data === 'string' && data) err.message = data
    return Promise.reject(err)
  },
)

// errMsg extracts a human-readable message from a rejected request.
export function errMsg(err, fallback = '请求失败') {
  return err?.message || fallback
}

export default api
