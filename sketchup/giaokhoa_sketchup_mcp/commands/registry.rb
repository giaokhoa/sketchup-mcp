# frozen_string_literal: true

module Giaokhoa
  module SketchupMcp
    module Commands
      class Registry
        OPERATIONS = ['system.ping', 'session.info'].freeze

        def initialize(session_id:, model_state:)
          @session_id = session_id
          @model_state = model_state
        end

        def call(operation, payload)
          return invalid('payload must be an empty object') unless payload.empty?

          case operation
          when 'system.ping'
            ok('pong' => true)
          when 'session.info'
            snapshot = @model_state.snapshot
            ok(
              'session_id' => @session_id,
              'pid' => Process.pid,
              'sketchup_version' => Sketchup.version,
              'model' => {
                'guid' => snapshot.fetch(:guid),
                'title' => snapshot.fetch(:title),
                'revision' => snapshot.fetch(:revision)
              }
            )
          else
            invalid("unsupported operation: #{operation}")
          end
        end

        private

        def ok(payload)
          {ok: true, payload: payload}
        end

        def invalid(message)
          {
            ok: false,
            error: {
              code: 'INVALID_REQUEST',
              message: message
            }
          }
        end
      end
    end
  end
end
