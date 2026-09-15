# frozen_string_literal: true

require 'securerandom'

module Giaokhoa
  module SketchupMcp
    class LifecycleObserver < Sketchup::AppObserver
      def initialize(runtime)
        super()
        @runtime = runtime
      end

      def expectsStartupModelNotifications
        true
      end

      def onNewModel(model)
        @runtime.track_model(model)
      end

      def onOpenModel(model)
        @runtime.track_model(model)
      end

      def onActivateModel(model)
        @runtime.track_model(model)
      end

      def onQuit
        @runtime.stop
      end

      def onUnloadExtension(extension_name)
        @runtime.stop if extension_name == 'Giaokhoa SketchUp MCP'
      end
    end

    module Runtime
      module_function

      def start
        return if @server&.running?

        @session_id = SecureRandom.uuid
        @token = SecureRandom.urlsafe_base64(32, false)
        @model_state = ModelState.new
        @model_state.track(Sketchup.active_model)

        discovery = Bridge::Discovery.new(session_id: @session_id, token: @token)
        @server = Bridge::Server.new(
          session_id: @session_id,
          token: @token,
          discovery: discovery
        )
        registry = Commands::Registry.new(session_id: @session_id, model_state: @model_state)
        @dispatcher = Dispatcher::MainThreadDispatcher.new(
          request_queue: @server.request_queue,
          command_registry: registry,
          ui: UI
        )

        @server.start
        @dispatcher.start
        @lifecycle_observer = LifecycleObserver.new(self)
        Sketchup.add_observer(@lifecycle_observer)
        nil
      rescue StandardError
        stop
        raise
      end

      def stop
        safely { @dispatcher&.stop }
        safely { @server&.stop }
        safely { @model_state&.stop }
        if @lifecycle_observer
          observer = @lifecycle_observer
          @lifecycle_observer = nil
          safely { Sketchup.remove_observer(observer) }
        end
        @dispatcher = nil
        @server = nil
        @model_state = nil
        @session_id = nil
        @token = nil
        nil
      end

      def safely
        yield
      rescue StandardError
        nil
      end
      private_class_method :safely

      def track_model(model)
        @model_state&.track(model)
      end
    end
  end
end
