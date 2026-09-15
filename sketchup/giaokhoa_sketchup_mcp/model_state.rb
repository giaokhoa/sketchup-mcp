# frozen_string_literal: true

module Giaokhoa
  module SketchupMcp
    class ModelState
      class Observer < Sketchup::ModelObserver
        def initialize(owner)
          super()
          @owner = owner
        end

        def onTransactionCommit(model)
          @owner.transaction_changed(model)
        end

        def onTransactionUndo(model)
          @owner.transaction_changed(model)
        end

        def onTransactionRedo(model)
          @owner.transaction_changed(model)
        end
      end

      def initialize
        @model = nil
        @guid = nil
        @revision = 0
        @observer = Observer.new(self)
      end

      def track(model)
        guid = model.guid
        return if @model.equal?(model) && @guid == guid

        detach
        @model = model
        @guid = guid
        @revision = 0
        @model.add_observer(@observer)
        nil
      end

      def snapshot
        model = Sketchup.active_model
        track(model) unless @model.equal?(model) && @guid == model.guid
        {
          guid: @guid,
          title: model.title,
          revision: @revision
        }
      end

      def transaction_changed(model)
        return unless @model.equal?(model)

        current_guid = model.guid
        if current_guid != @guid
          @guid = current_guid
          @revision = 0
        else
          @revision += 1
        end
        nil
      end

      def stop
        detach
      end

      private

      def detach
        return unless @model

        @model.remove_observer(@observer)
      rescue StandardError
        nil
      ensure
        @model = nil
      end
    end
  end
end
