import { useMemo, useState } from 'react'
import { useNavigate, useLocation, Navigate } from 'react-router-dom'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Loader2 } from 'lucide-react'

import { useAuth } from '@/auth/useAuth'
import { ApiError } from '@/lib/api'
import { useT } from '@/lib/i18n'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'

interface FormValues {
  email: string
  password: string
}

interface LocationState {
  from?: string
}

export function LoginPage() {
  const navigate = useNavigate()
  const location = useLocation()
  const { token, login } = useAuth()
  const { t } = useT()
  const [serverError, setServerError] = useState<string | null>(null)

  // Schema rebuilt per locale so zod validation messages are localized (zod is
  // UX only; the Go backend is the real gate).
  const schema = useMemo(
    () =>
      z.object({
        email: z.string().email(t('auth.emailInvalid')),
        password: z.string().min(1, t('auth.passwordRequired')),
      }),
    [t],
  )

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { email: '', password: '' },
  })

  // Already authenticated → skip the login screen.
  if (token) {
    const from = (location.state as LocationState | null)?.from
    return <Navigate to={from && from.startsWith('/dashboard') ? from : '/dashboard'} replace />
  }

  async function onSubmit(values: FormValues) {
    setServerError(null)
    try {
      await login(values.email, values.password)
      const from = (location.state as LocationState | null)?.from
      navigate(from && from.startsWith('/dashboard') ? from : '/dashboard', {
        replace: true,
      })
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setServerError(t('auth.invalid'))
      } else if (err instanceof ApiError) {
        setServerError(err.message)
      } else {
        setServerError(t('common.error'))
      }
    }
  }

  const submitting = form.formState.isSubmitting

  return (
    <div className="flex flex-1 items-center justify-center px-4 py-12">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>{t('auth.loginTitle')}</CardTitle>
          <CardDescription>{t('auth.loginSubtitle')}</CardDescription>
        </CardHeader>
        <CardContent>
          <Form {...form}>
            <form onSubmit={form.handleSubmit(onSubmit)} className="flex flex-col gap-4">
              <FormField
                control={form.control}
                name="email"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('auth.email')}</FormLabel>
                    <FormControl>
                      <Input
                        type="email"
                        autoComplete="email"
                        placeholder={t('auth.emailPlaceholder')}
                        disabled={submitting}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="password"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('auth.password')}</FormLabel>
                    <FormControl>
                      <Input
                        type="password"
                        autoComplete="current-password"
                        disabled={submitting}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {serverError && (
                <p className="text-sm text-destructive" role="alert">
                  {serverError}
                </p>
              )}

              <Button type="submit" className="w-full" disabled={submitting}>
                {submitting && <Loader2 className="h-4 w-4 animate-spin" />}
                {submitting ? t('auth.signingIn') : t('auth.signIn')}
              </Button>
            </form>
          </Form>
        </CardContent>
      </Card>
    </div>
  )
}
