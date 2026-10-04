import type { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode, TextareaHTMLAttributes } from 'react'
import { cn } from '../lib/cn.ts'

export function Button(props: ButtonHTMLAttributes<HTMLButtonElement> & { tone?: 'primary' | 'quiet' | 'danger' }) {
  const { className, tone = 'primary', type = 'button', ...rest } = props
  return (
    <button
      type={type}
      className={cn(
        'inline-flex items-center justify-center rounded-md px-3 py-2 text-sm disabled:opacity-50',
        tone === 'primary' && 'bg-[#1c1915] text-[#f4efe6]',
        tone === 'quiet' && 'border border-[#1c1915]/20 bg-white',
        tone === 'danger' && 'bg-[#8f3d2c] text-white',
        className,
      )}
      {...rest}
    />
  )
}

export function Input(props: InputHTMLAttributes<HTMLInputElement>) {
  const { className, ...rest } = props
  return <input className={cn('w-full rounded-md border border-[#1c1915]/20 bg-white px-3 py-2 text-sm', className)} {...rest} />
}

export function TextArea(props: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  const { className, ...rest } = props
  return <textarea className={cn('w-full rounded-md border border-[#1c1915]/20 bg-white px-3 py-2 text-sm', className)} {...rest} />
}

export function Card(props: { title?: string; children: ReactNode; className?: string }) {
  return (
    <section className={cn('rounded-lg border border-[#1c1915]/10 bg-white/80 p-4', props.className)}>
      {props.title ? <h2 className="mb-3 text-base font-semibold">{props.title}</h2> : null}
      {props.children}
    </section>
  )
}

export function Notice(props: { children: ReactNode; tone?: 'plain' | 'warn' }) {
  return (
    <p className={cn('rounded-md px-3 py-2 text-sm', props.tone === 'warn' ? 'bg-[#f3e1c8]' : 'bg-[#efeae2]')}>
      {props.children}
    </p>
  )
}
