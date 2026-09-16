# frozen_string_literal: true

module Giaokhoa
  module SketchupMcp
    module Dispatcher
      class MainThreadDispatcher
        DEFAULT_INTERVAL = 0.01
        DEFAULT_MAX_PER_TICK = 8

        def initialize(request_queue:, command_registry:, ui:, interval: DEFAULT_INTERVAL,
                       max_per_tick: DEFAULT_MAX_PER_TICK, monotonic_clock: nil)
          @request_queue = request_queue
          @command_registry = command_registry
          @ui = ui
          @interval = interval
          @max_per_tick = max_per_tick
          @monotonic_clock = monotonic_clock || -> { Process.clock_gettime(Process::CLOCK_MONOTONIC) }
          @timer_id = nil
        end

        def start
          return if @timer_id

          @timer_id = @ui.start_timer(@interval, true) { drain_once }
          nil
        end

        def stop
          return unless @timer_id

          @ui.stop_timer(@timer_id)
          @timer_id = nil
          nil
        end

        def drain_once
          processed = 0
          while processed < @max_per_tick
            request = pop_nonblocking
            break unless request

            processed += 1
            process(request)
          end
          processed
        end

        private

        def pop_nonblocking
          @request_queue.pop(true)
        rescue ThreadError
          nil
        end

        def process(request)
          return if request.cancelled?

          if @monotonic_clock.call >= request.deadline
            request.response_queue << Bridge::Protocol.error_response(
              request.id,
              'TIMEOUT',
              'request timed out before main-thread execution'
            )
            return
          end

          result = @command_registry.call(request.operation, request.payload)
          response = if result.fetch(:ok)
                       Bridge::Protocol.success_response(request.id, result.fetch(:payload))
                     else
                       error = result.fetch(:error)
                       Bridge::Protocol.error_response(
                         request.id,
                         error.fetch('code'),
                         error.fetch('message'),
                         error['details']
                       )
                     end
          request.response_queue << response
        rescue StandardError
          request.response_queue << Bridge::Protocol.error_response(
            request.id,
            'INTERNAL',
            'command execution failed'
          ) unless request.cancelled?
        end
      end
    end
  end
end
